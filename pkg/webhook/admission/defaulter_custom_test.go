/*
Copyright 2021 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package admission

import (
	"bytes"
	"context"
	"maps"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

var _ = Describe("Defaulter Handler", func() {

	It("should remove unknown fields when DefaulterRemoveUnknownFields is passed and some fields are defaulted", func(ctx SpecContext) {
		obj := &TestDefaulter{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{}, DefaulterRemoveUnknownOrOmitableFields)

		resp := handler.Handle(ctx, Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: []byte(`{"newField":"foo", "totalReplicas":5}`),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).To(HaveLen(4))
		Expect(resp.Patches).To(ContainElements(
			jsonpatch.JsonPatchOperation{
				Operation: "add",
				Path:      "/labels",
				Value:     map[string]any{"foo": "bar"},
			},
			jsonpatch.JsonPatchOperation{
				Operation: "add",
				Path:      "/replica",
				Value:     2.0,
			},
			jsonpatch.JsonPatchOperation{
				Operation: "remove",
				Path:      "/newField",
			},
			jsonpatch.JsonPatchOperation{
				Operation: "remove",
				Path:      "/totalReplicas",
			},
		))
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})

	It("should remove unknown fields when DefaulterRemoveUnknownFields is passed and no fields are defaulted", func(ctx SpecContext) {
		obj := &TestDefaulter{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{}, DefaulterRemoveUnknownOrOmitableFields)

		resp := handler.Handle(ctx, Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: []byte(`{"labels":{"foo": "bar"}, "replica": 2, "totalReplicas":0}`),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).To(HaveLen(1))
		Expect(resp.Patches).To(ContainElements(
			jsonpatch.JsonPatchOperation{
				Operation: "remove",
				Path:      "/totalReplicas",
			},
		))
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})

	It("should preserve unknown fields by default", func(ctx SpecContext) {
		obj := &TestDefaulter{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{})

		resp := handler.Handle(ctx, Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: []byte(`{"newField":"foo", "totalReplicas":5}`),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Patches).To(HaveLen(3))
		Expect(resp.Patches).To(ContainElements(
			jsonpatch.JsonPatchOperation{
				Operation: "add",
				Path:      "/labels",
				Value:     map[string]any{"foo": "bar"},
			},
			jsonpatch.JsonPatchOperation{
				Operation: "add",
				Path:      "/replica",
				Value:     2.0,
			},
			jsonpatch.JsonPatchOperation{
				Operation: "remove",
				Path:      "/totalReplicas",
			},
		))
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})

	It("should return ok if received delete verb in defaulter handler", func(ctx SpecContext) {
		obj := &TestDefaulter{}
		handler := WithCustomDefaulter(admissionScheme, obj, &TestCustomDefaulter{})
		resp := handler.Handle(ctx, Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Delete,
				OldObject: runtime.RawExtension{
					Raw: []byte("{}"),
				},
			},
		})
		Expect(resp.Allowed).Should(BeTrue())
		Expect(resp.Result.Code).Should(Equal(int32(http.StatusOK)))
	})
})

// TestDefaulter.
var _ runtime.Object = &TestDefaulter{}

type TestDefaulter struct {
	Labels map[string]string `json:"labels,omitempty"`

	Replica       int `json:"replica,omitempty"`
	TotalReplicas int `json:"totalReplicas,omitempty"`
}

var testDefaulterGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestDefaulter"}

func (d *TestDefaulter) GetObjectKind() schema.ObjectKind { return d }
func (d *TestDefaulter) DeepCopyObject() runtime.Object {
	return &TestDefaulter{
		Labels:        maps.Clone(d.Labels),
		Replica:       d.Replica,
		TotalReplicas: d.TotalReplicas,
	}
}

func (d *TestDefaulter) GroupVersionKind() schema.GroupVersionKind {
	return testDefaulterGVK
}

func (d *TestDefaulter) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestDefaulterList{}

type TestDefaulterList struct{}

func (*TestDefaulterList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestDefaulterList) DeepCopyObject() runtime.Object   { return nil }

// TestCustomDefaulter
type TestCustomDefaulter struct{}

func (d *TestCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	o := obj.(*TestDefaulter)

	if o.Labels == nil {
		o.Labels = map[string]string{}
	}
	o.Labels["foo"] = "bar"

	if o.Replica < 2 {
		o.Replica = 2
	}
	o.TotalReplicas = 0
	return nil
}

func BenchmarkDefaulter(b *testing.B) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	b.Run("noop", func(b *testing.B) {
		req := createTestRequest("test-hostname")
		handler := WithDefaulter(scheme, &PodDefaulter{
			defaultHostname: "test-hostname", // defaulting will be no-op as hostname is already "test-hostname"
		})
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = handler.Handle(b.Context(), req)
		}
	})

	b.Run("with changes", func(b *testing.B) {
		req := createTestRequest("")
		handler := WithDefaulter(scheme, &PodDefaulter{
			defaultHostname: "test-hostname", // will be used to default hostname
		})
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = handler.Handle(b.Context(), req)
		}
	})
}

type PodDefaulter struct {
	defaultHostname string
}

func (p PodDefaulter) Default(_ context.Context, pod *corev1.Pod) error {
	if pod.Spec.Hostname == "" && p.defaultHostname != "" {
		pod.Spec.Hostname = p.defaultHostname
	}
	return nil
}

func createTestRequest(hostname string) Request {
	return Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: "12345",
			Kind: metav1.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
			Object:    runtime.RawExtension{Raw: mustEncodeLikeAPIServer(createSuperPod(hostname))},
			Operation: admissionv1.Create,
		},
	}
}

func createSuperPod(hostname string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "monolith-service-7f8d9b",
			Namespace: "prod",
			Labels: map[string]string{
				"app": "processor", "env": "prod", "version": "1.4.2",
				"pci-compliant": "true", "team": "data-eng",
			},
			Annotations: map[string]string{
				"agent-inject":    "true",
				"checksum/config": "e3b0c23238fc1c149afbf4c8996fb92427ae",
			},
			Finalizers: []string{"cleanup.finalizer.my.io"},
		},
		Spec: corev1.PodSpec{
			Hostname: hostname,
			InitContainers: []corev1.Container{
				{
					Name: "vault-init", Image: "vault:1.13.1",
					SecurityContext: &corev1.SecurityContext{RunAsUser: ptr.To[int64](1000)},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "main-app",
					Image: "my-priv-reg.io/analytics:v1.4.2",
					Env: []corev1.EnvVar{
						{Name: "DB_URL", Value: "postgres://db:5432"},
						{
							Name: "POD_IP",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
							},
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(8080)},
						},
						InitialDelaySeconds: 30,
					},
				},
				{
					Name:  "log-shipper",
					Image: "fluentbit:2.1.4",
					Args:  []string{"--config", "/etc/fluent-bit/fluent-bit.conf"},
				},
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{Key: "topology.kubernetes.io/zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1a", "us-east-1b"}},
								},
							},
						},
					},
				},
			},
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
				{
					MaxSkew:           1,
					TopologyKey:       "kubernetes.io/hostname",
					WhenUnsatisfiable: corev1.DoNotSchedule,
					LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "processor"}},
				},
			},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   ptr.To(true),
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			TerminationGracePeriodSeconds: ptr.To[int64](60),
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: "target-health.lbv3.k8s.cloud/my-tg"},
			},
			Volumes: []corev1.Volume{
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
						},
					},
				},
			},
		},
	}
}

func mustEncodeLikeAPIServer(obj runtime.Object) []byte {
	// 1. Create a NewSerializer
	// Options: false (not pretty printed), false (not YAML)
	s := json.NewSerializerWithOptions(
		json.DefaultMetaFactory,
		nil,
		nil,
		json.SerializerOptions{Yaml: false, Pretty: false, Strict: false},
	)

	// 2. Use a Buffer for efficiency
	buf := new(bytes.Buffer)
	err := s.Encode(obj, buf)
	if err != nil {
		panic(err)
	}

	return buf.Bytes()
}
