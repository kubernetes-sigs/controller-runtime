/*
Copyright The Kubernetes Authors.

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

package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("Delete response", func() {
	const (
		ns        = "default"
		finalizer = "example.com/hold"
	)
	var counter atomic.Uint64
	configMapGVK := corev1.SchemeGroupVersion.WithKind("ConfigMap")

	type representation struct {
		name     string
		toDelete func(cm *corev1.ConfigMap) client.Object
		gvk      schema.GroupVersionKind
	}

	representations := []representation{{
		name:     "typed",
		toDelete: func(cm *corev1.ConfigMap) client.Object { return cm.DeepCopy() },
	}, {
		name: "unstructured",
		toDelete: func(cm *corev1.ConfigMap) client.Object {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(configMapGVK)
			u.SetNamespace(cm.Namespace)
			u.SetName(cm.Name)
			u.SetResourceVersion(cm.ResourceVersion)
			return u
		},
		gvk: configMapGVK,
	}, {
		name: "metadata",
		toDelete: func(cm *corev1.ConfigMap) client.Object {
			m := &metav1.PartialObjectMetadata{}
			m.SetGroupVersionKind(configMapGVK)
			m.SetNamespace(cm.Namespace)
			m.SetName(cm.Name)
			m.SetResourceVersion(cm.ResourceVersion)
			return m
		},
		gvk: configMapGVK,
	}}

	newConfigMap := func(ctx context.Context, finalizers ...string) *corev1.ConfigMap {
		cm := &corev1.ConfigMap{
			Name:       fmt.Sprintf("delete-response-%d", counter.Add(1)),
			Namespace:  ns,
			Finalizers: finalizers,
			Data:       map[string]string{"key": "value"},
		}
		cm, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func(ctx context.Context) {
			_, err := clientset.CoreV1().ConfigMaps(ns).Patch(ctx, cm.Name, "application/merge-patch+json",
				[]byte(`{"metadata":{"finalizers":null}}`), metav1.PatchOptions{})
			Expect(client.IgnoreNotFound(err)).To(Succeed())
			err = clientset.CoreV1().ConfigMaps(ns).Delete(ctx, cm.Name, metav1.DeleteOptions{})
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
		return cm
	}

	getConfigMap := func(ctx context.Context, name string) (*corev1.ConfigMap, error) {
		return clientset.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	}

	var cl client.WithWatch
	BeforeEach(func() {
		var err error
		cl, err = client.NewWithWatch(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())
	})

	for _, repr := range representations {
		Context("with "+repr.name+" objects", func() {
			It("should leave obj unchanged if the object was removed from storage", func(ctx SpecContext) {
				cm := newConfigMap(ctx)
				obj := repr.toDelete(cm)
				before := obj.DeepCopyObject()

				Expect(cl.Delete(ctx, obj)).To(Succeed())
				Expect(obj).To(Equal(before))

				_, err := getConfigMap(ctx, cm.Name)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("should update obj with the terminating object if it is held by finalizers", func(ctx SpecContext) {
				cm := newConfigMap(ctx, finalizer)
				obj := repr.toDelete(cm)

				Expect(cl.Delete(ctx, obj)).To(Succeed())

				stored, err := getConfigMap(ctx, cm.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(stored.DeletionTimestamp).NotTo(BeNil())

				Expect(obj.GetObjectKind().GroupVersionKind()).To(Equal(repr.gvk))
				Expect(obj.GetUID()).To(Equal(cm.UID))
				Expect(obj.GetDeletionTimestamp()).NotTo(BeNil())
				Expect(obj.GetFinalizers()).To(ConsistOf(finalizer))
				Expect(obj.GetResourceVersion()).To(Equal(stored.ResourceVersion))
				Expect(obj.GetResourceVersion()).NotTo(Equal(cm.ResourceVersion))
			})

			It("should update obj without changing its resourceVersion if it was already terminating", func(ctx SpecContext) {
				cm := newConfigMap(ctx, finalizer)
				Expect(cl.Delete(ctx, repr.toDelete(cm))).To(Succeed())
				terminating, err := getConfigMap(ctx, cm.Name)
				Expect(err).NotTo(HaveOccurred())

				obj := repr.toDelete(terminating)
				obj.SetLabels(map[string]string{"local": "change"})
				Expect(cl.Delete(ctx, obj)).To(Succeed())
				Expect(obj.GetLabels()).NotTo(HaveKey("local"))
				Expect(obj.GetResourceVersion()).To(Equal(terminating.ResourceVersion))
				Expect(obj.GetDeletionTimestamp()).To(Equal(terminating.DeletionTimestamp))
			})

			It("should update obj with the object as it would remain on a dry-run delete", func(ctx SpecContext) {
				cm := newConfigMap(ctx, finalizer)
				obj := repr.toDelete(cm)

				Expect(cl.Delete(ctx, obj, client.DryRunAll)).To(Succeed())
				Expect(obj.GetObjectKind().GroupVersionKind()).To(Equal(repr.gvk))
				Expect(obj.GetDeletionTimestamp()).NotTo(BeNil())
				Expect(obj.GetResourceVersion()).To(Equal(cm.ResourceVersion), "a dry-run does not persist a new resourceVersion")

				stored, err := getConfigMap(ctx, cm.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(stored.DeletionTimestamp).To(BeNil())
			})

			It("should leave obj unchanged on a dry-run delete of an object without finalizers", func(ctx SpecContext) {
				cm := newConfigMap(ctx)
				obj := repr.toDelete(cm)
				before := obj.DeepCopyObject()

				Expect(cl.Delete(ctx, obj, client.DryRunAll)).To(Succeed())
				Expect(obj).To(Equal(before))

				_, err := getConfigMap(ctx, cm.Name)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should leave obj unchanged if a precondition fails", func(ctx SpecContext) {
				cm := newConfigMap(ctx, finalizer)
				obj := repr.toDelete(cm)
				before := obj.DeepCopyObject()

				err := cl.Delete(ctx, obj, client.Preconditions{ResourceVersion: new("1")})
				Expect(apierrors.IsConflict(err)).To(BeTrue(), "expected a conflict, got %v", err)
				Expect(obj).To(Equal(before))
			})

			It("should leave obj unchanged if the object does not exist", func(ctx SpecContext) {
				obj := repr.toDelete(&corev1.ConfigMap{Name: "delete-response-does-not-exist", Namespace: ns})
				before := obj.DeepCopyObject()

				err := cl.Delete(ctx, obj)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound, got %v", err)
				Expect(obj).To(Equal(before))
			})
		})
	}

	It("should update all fields of typed objects", func(ctx SpecContext) {
		dep := &appsv1.Deployment{
			Name:       fmt.Sprintf("delete-response-%d", counter.Add(1)),
			Namespace:  ns,
			Finalizers: []string{finalizer},
			Spec: appsv1.DeploymentSpec{
				Replicas: new(int32(3)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "delete-response"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "delete-response"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
				},
			},
		}
		Expect(cl.Create(ctx, dep)).To(Succeed())
		DeferCleanup(func(ctx context.Context) {
			_, err := clientset.AppsV1().Deployments(ns).Patch(ctx, dep.Name, "application/merge-patch+json",
				[]byte(`{"metadata":{"finalizers":null}}`), metav1.PatchOptions{})
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})

		Expect(cl.Delete(ctx, dep)).To(Succeed())

		stored, err := clientset.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(dep).To(Equal(stored))
	})

	It("should preserve the GVK set on typed objects", func(ctx SpecContext) {
		cm := newConfigMap(ctx, finalizer)
		obj := cm.DeepCopy()
		obj.SetGroupVersionKind(configMapGVK)

		Expect(cl.Delete(ctx, obj)).To(Succeed())
		Expect(obj.GetDeletionTimestamp()).NotTo(BeNil())
		Expect(obj.GroupVersionKind()).To(Equal(configMapGVK))
	})

	It("should update metadata objects of kinds that are not in the scheme", func(ctx SpecContext) {
		s := runtime.NewScheme()
		Expect(kscheme.AddToScheme(s)).To(Succeed())
		cl, err := client.New(cfg, client.Options{Scheme: s})
		Expect(err).NotTo(HaveOccurred())

		chaosPodGVK := schema.GroupVersionKind{Group: "chaosapps.metamagical.io", Version: "v1", Kind: "ChaosPod"}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(chaosPodGVK)
		u.SetNamespace(ns)
		u.SetName(fmt.Sprintf("delete-response-%d", counter.Add(1)))
		u.SetFinalizers([]string{finalizer})
		Expect(cl.Create(ctx, u)).To(Succeed())
		DeferCleanup(func(ctx context.Context) {
			Expect(client.IgnoreNotFound(cl.Patch(ctx, u, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":null}}`))))).To(Succeed())
		})

		m := &metav1.PartialObjectMetadata{}
		m.SetGroupVersionKind(chaosPodGVK)
		m.SetNamespace(ns)
		m.SetName(u.GetName())
		Expect(cl.Delete(ctx, m)).To(Succeed())
		Expect(m.GroupVersionKind()).To(Equal(chaosPodGVK))
		Expect(m.GetDeletionTimestamp()).NotTo(BeNil())
		Expect(m.GetFinalizers()).To(ConsistOf(finalizer))
	})

	DescribeTable("should update obj through client wrappers",
		func(ctx SpecContext, wrap func(client.WithWatch) client.Client, dryRun bool) {
			cm := newConfigMap(ctx, finalizer)
			obj := cm.DeepCopy()

			Expect(wrap(cl).Delete(ctx, obj)).To(Succeed())
			Expect(obj.DeletionTimestamp).NotTo(BeNil())

			stored, err := getConfigMap(ctx, cm.Name)
			Expect(err).NotTo(HaveOccurred())
			if dryRun {
				Expect(stored.DeletionTimestamp).To(BeNil())
			} else {
				Expect(obj).To(Equal(stored))
			}
		},
		Entry("namespaced", func(c client.WithWatch) client.Client { return client.NewNamespacedClient(c, ns) }, false),
		Entry("field owner", func(c client.WithWatch) client.Client { return client.WithFieldOwner(c, "test") }, false),
		Entry("field validation", func(c client.WithWatch) client.Client {
			return client.WithFieldValidation(c, metav1.FieldValidationStrict)
		}, false),
		Entry("dry-run", func(c client.WithWatch) client.Client { return client.NewDryRunClient(c) }, true),
		Entry("interceptor", func(c client.WithWatch) client.Client {
			return interceptor.NewClient(c, interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					return c.Delete(ctx, obj, opts...)
				},
			})
		}, false),
	)
})

var _ = Describe("Delete response for pods", func() {
	const (
		ns        = "default"
		finalizer = "example.com/hold"
	)
	var counter atomic.Uint64
	podGVK := corev1.SchemeGroupVersion.WithKind("Pod")

	namespaceGVK := corev1.SchemeGroupVersion.WithKind("Namespace")

	representations := []struct {
		name     string
		toDelete func(obj client.Object, gvk schema.GroupVersionKind) client.Object
	}{{
		name: "typed",
		toDelete: func(obj client.Object, _ schema.GroupVersionKind) client.Object {
			return obj.DeepCopyObject().(client.Object)
		},
	}, {
		name: "unstructured",
		toDelete: func(obj client.Object, gvk schema.GroupVersionKind) client.Object {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(gvk)
			u.SetNamespace(obj.GetNamespace())
			u.SetName(obj.GetName())
			u.SetResourceVersion(obj.GetResourceVersion())
			return u
		},
	}, {
		name: "metadata",
		toDelete: func(obj client.Object, gvk schema.GroupVersionKind) client.Object {
			m := &metav1.PartialObjectMetadata{}
			m.SetGroupVersionKind(gvk)
			m.SetNamespace(obj.GetNamespace())
			m.SetName(obj.GetName())
			m.SetResourceVersion(obj.GetResourceVersion())
			return m
		},
	}}

	newPod := func(ctx context.Context, nodeName string, finalizers ...string) *corev1.Pod {
		pod := &corev1.Pod{
			Name:       fmt.Sprintf("delete-response-pod-%d", counter.Add(1)),
			Namespace:  ns,
			Finalizers: finalizers,
			Spec: corev1.PodSpec{
				NodeName:   nodeName,
				Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
			},
		}
		pod, err := clientset.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func(ctx context.Context) {
			_, err := clientset.CoreV1().Pods(ns).Patch(ctx, pod.Name, "application/merge-patch+json",
				[]byte(`{"metadata":{"finalizers":null}}`), metav1.PatchOptions{})
			Expect(client.IgnoreNotFound(err)).To(Succeed())
			err = clientset.CoreV1().Pods(ns).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: new(int64(0))})
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
		return pod
	}

	getPod := func(ctx context.Context, name string) (*corev1.Pod, error) {
		return clientset.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	}

	var cl client.Client
	BeforeEach(func() {
		var err error
		cl, err = client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())
	})

	for _, repr := range representations {
		Context("with "+repr.name+" objects", func() {
			It("should leave obj unchanged if an unbound pod was removed from storage", func(ctx SpecContext) {
				pod := newPod(ctx, "")
				obj := repr.toDelete(pod, podGVK)
				before := obj.DeepCopyObject()

				Expect(cl.Delete(ctx, obj)).To(Succeed())
				Expect(obj).To(Equal(before))

				_, err := getPod(ctx, pod.Name)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("should update obj if a bound pod remains for its grace period", func(ctx SpecContext) {
				pod := newPod(ctx, "delete-response-node")
				obj := repr.toDelete(pod, podGVK)

				Expect(cl.Delete(ctx, obj)).To(Succeed())
				Expect(obj.GetDeletionTimestamp()).NotTo(BeNil())
				Expect(obj.GetDeletionGracePeriodSeconds()).To(HaveValue(BeNumerically(">", 0)))

				stored, err := getPod(ctx, pod.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(obj.GetResourceVersion()).To(Equal(stored.ResourceVersion))
			})

			It("should leave obj unchanged on a dry-run delete of an unbound pod", func(ctx SpecContext) {
				pod := newPod(ctx, "")
				obj := repr.toDelete(pod, podGVK)
				before := obj.DeepCopyObject()

				Expect(cl.Delete(ctx, obj, client.DryRunAll)).To(Succeed())
				Expect(obj).To(Equal(before))

				_, err := getPod(ctx, pod.Name)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should update obj on a dry-run delete of a bound pod", func(ctx SpecContext) {
				pod := newPod(ctx, "delete-response-node")
				obj := repr.toDelete(pod, podGVK)

				Expect(cl.Delete(ctx, obj, client.DryRunAll)).To(Succeed())
				Expect(obj.GetDeletionTimestamp()).NotTo(BeNil())

				stored, err := getPod(ctx, pod.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(stored.DeletionTimestamp).To(BeNil())
			})

			It("should update obj if a namespace remains until its spec finalizers are removed", func(ctx SpecContext) {
				namespace := &corev1.Namespace{Name: fmt.Sprintf("delete-response-ns-%d", counter.Add(1))}
				Expect(cl.Create(ctx, namespace)).To(Succeed())
				DeferCleanup(func(ctx context.Context) { deleteNamespace(ctx, namespace) })
				obj := repr.toDelete(namespace, namespaceGVK)
				gvk := obj.GetObjectKind().GroupVersionKind()

				Expect(cl.Delete(ctx, obj)).To(Succeed())
				Expect(obj.GetDeletionTimestamp()).NotTo(BeNil())
				Expect(obj.GetObjectKind().GroupVersionKind()).To(Equal(gvk))
			})
		})
	}

	It("should update obj if a pod is held by finalizers", func(ctx SpecContext) {
		pod := newPod(ctx, "", finalizer)
		obj := pod.DeepCopy()

		Expect(cl.Delete(ctx, obj)).To(Succeed())
		Expect(obj.DeletionTimestamp).NotTo(BeNil())
		Expect(obj.Finalizers).To(ConsistOf(finalizer))

		stored, err := getPod(ctx, pod.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(obj).To(Equal(stored))
	})

	It("should leave obj unchanged if a terminating pod is removed by a delete without grace period", func(ctx SpecContext) {
		pod := newPod(ctx, "delete-response-node")
		Expect(cl.Delete(ctx, pod)).To(Succeed())
		Expect(pod.DeletionTimestamp).NotTo(BeNil())
		before := pod.DeepCopy()

		Expect(cl.Delete(ctx, pod, client.GracePeriodSeconds(0))).To(Succeed())
		Expect(pod).To(Equal(before))

		_, err := getPod(ctx, pod.Name)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})

type nillableDuration struct {
	*time.Duration

	Raw []byte
}

func (d *nillableDuration) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	pd, err := time.ParseDuration(str)
	if err != nil {
		return err
	}
	d.Raw = slices.Clone(b)
	d.Duration = &pd
	return nil
}

func (d nillableDuration) MarshalJSON() ([]byte, error) {
	if d.Raw != nil {
		return d.Raw, nil
	}
	return json.Marshal(d.String())
}

func (d nillableDuration) ToUnstructured() any {
	if d.Raw != nil {
		return d.Raw
	}
	return d.String()
}

type timerSpec struct {
	Duration nillableDuration `json:"duration"`
}

type timer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec timerSpec `json:"spec"`
}

func (t *timer) DeepCopyObject() runtime.Object {
	out := *t
	t.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if t.Spec.Duration.Duration != nil {
		d := *t.Spec.Duration.Duration
		out.Spec.Duration.Duration = &d
	}
	out.Spec.Duration.Raw = slices.Clone(t.Spec.Duration.Raw)
	return &out
}

var _ = Describe("Delete response for types with custom JSON marshalling", func() {
	const (
		ns        = "default"
		finalizer = "example.com/hold"
	)
	var counter atomic.Uint64
	chaosPodGV := schema.GroupVersion{Group: "chaosapps.metamagical.io", Version: "v1"}

	var cl client.Client
	BeforeEach(func() {
		s := runtime.NewScheme()
		s.AddKnownTypeWithName(chaosPodGV.WithKind("ChaosPod"), &timer{})
		metav1.AddToGroupVersion(s, chaosPodGV)
		var err error
		cl, err = client.New(cfg, client.Options{Scheme: s})
		Expect(err).NotTo(HaveOccurred())
	})

	newTimer := func(ctx context.Context, finalizers ...string) *timer {
		t := &timer{
			Name:       fmt.Sprintf("delete-response-timer-%d", counter.Add(1)),
			Namespace:  ns,
			Finalizers: finalizers,
		}
		Expect(json.Unmarshal([]byte(`"30s"`), &t.Spec.Duration)).To(Succeed())
		Expect(cl.Create(ctx, t)).To(Succeed())
		DeferCleanup(func(ctx context.Context) {
			patch := client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":null}}`))
			Expect(client.IgnoreNotFound(cl.Patch(ctx, t.DeepCopyObject().(client.Object), patch))).To(Succeed())
		})
		return t
	}

	It("should update obj if the object is held by finalizers", func(ctx SpecContext) {
		t := newTimer(ctx, finalizer)

		Expect(cl.Delete(ctx, t)).To(Succeed())
		Expect(t.DeletionTimestamp).NotTo(BeNil())
		Expect(t.Spec.Duration.Duration).To(HaveValue(Equal(30 * time.Second)))
	})

	It("should leave obj unchanged if the object was removed from storage", func(ctx SpecContext) {
		t := newTimer(ctx)
		before := t.DeepCopyObject()

		Expect(cl.Delete(ctx, t)).To(Succeed())
		Expect(t).To(Equal(before))
	})
})
