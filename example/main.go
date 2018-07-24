/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/go-logr/logr"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

var log = logf.Log.WithName("example-controller")

func main() {
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(false))
	entryLog := log.WithName("entrypoint")

	// Setup a Manager
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	// Setup a new controller to Reconciler ReplicaSets
	c, err := controller.New("foo-controller", mgr, controller.Options{
		Reconciler: &reconcileReplicaSet{client: mgr.GetClient(), log: log.WithName("reconciler")},
	})
	if err != nil {
		entryLog.Error(err, "unable to set up individual controller")
		os.Exit(1)
	}

	// Watch ReplicaSets and enqueue ReplicaSet object key
	if err := c.Watch(&source.Kind{Type: &appsv1.ReplicaSet{}}, &handler.EnqueueRequestForObject{}); err != nil {
		entryLog.Error(err, "unable to watch ReplicaSets")
		os.Exit(1)
	}

	// Watch Pods and enqueue owning ReplicaSet key
	if err := c.Watch(&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestForOwner{OwnerType: &appsv1.ReplicaSet{}, IsController: true}); err != nil {
		entryLog.Error(err, "unable to watch Pods")
		os.Exit(1)
	}

	// Setup webhooks
	mutatingWebhook, err := builder.NewWebhookBuilder().
		Name("mutating.k8s.io").
		Type(types.WebhookTypeMutating).
		Path("/mutating-pods").
		Operations(admissionregistrationv1beta1.Create, admissionregistrationv1beta1.Update).
		WithManager(mgr).
		ForType(&corev1.Pod{}).
		Build(&podAnnotator{client: mgr.GetClient(), decoder: mgr.GetAdmissionDecoder()})
	if err != nil {
		entryLog.Error(err, "unable to setup mutating webhook")
		os.Exit(1)
	}

	validatingWebhook, err := builder.NewWebhookBuilder().
		Name("validating.k8s.io").
		Type(types.WebhookTypeValidating).
		Path("/validating-pods").
		Operations(admissionregistrationv1beta1.Create, admissionregistrationv1beta1.Update).
		WithManager(mgr).
		ForType(&corev1.Pod{}).
		Build(&podValidator{client: mgr.GetClient(), decoder: mgr.GetAdmissionDecoder()})
	if err != nil {
		entryLog.Error(err, "unable to setup validating webhook")
		os.Exit(1)
	}

	as, err := webhook.NewServer("foo-admission-server", mgr, webhook.ServerOptions{
		Port:    443,
		CertDir: "/tmp/cert",
		Client:  mgr.GetClient(),
		KVMap:   map[string]interface{}{"foo": "bar"},
		BootstrapOptions: &webhook.BootstrapOptions{
			Secret: &apitypes.NamespacedName{
				Namespace: "default",
				Name:      "foo-admission-server-secret",
			},

			Service: &apitypes.NamespacedName{
				Namespace: "default",
				Name:      "foo-admission-server-service",
			},
			// Labels should select the pods that runs this webhook server.
			Labels: map[string]string{
				"app": "foo-admission-server",
			},
		},
	})
	if err != nil {
		entryLog.Error(err, "unable to create a new webhook server")
		os.Exit(1)
	}
	err = as.Register(mutatingWebhook, validatingWebhook)
	if err != nil {
		entryLog.Error(err, "unable to register webhooks in the admission server")
		os.Exit(1)
	}

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}

// reconcileReplicaSet reconciles ReplicaSets
type reconcileReplicaSet struct {
	// client can be used to retrieve objects from the APIServer.
	client client.Client
	log    logr.Logger
}

// Implement reconcile.Reconciler so the controller can reconcile objects
var _ reconcile.Reconciler = &reconcileReplicaSet{}

func (r *reconcileReplicaSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// set up a convinient log object so we don't have to type request over and over again
	log := r.log.WithValues("request", request)

	// Fetch the ReplicaSet from the cache
	rs := &appsv1.ReplicaSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, rs)
	if errors.IsNotFound(err) {
		log.Error(nil, "Could not find ReplicaSet")
		return reconcile.Result{}, nil
	}

	if err != nil {
		log.Error(err, "Could not fetch ReplicaSet")
		return reconcile.Result{}, err
	}

	// Print the ReplicaSet
	log.Info("Reconciling ReplicaSet", "container name", rs.Spec.Template.Spec.Containers[0].Name)

	// Set the label if it is missing
	if rs.Labels == nil {
		rs.Labels = map[string]string{}
	}
	if rs.Labels["hello"] == "world" {
		return reconcile.Result{}, nil
	}

	// Update the ReplicaSet
	rs.Labels["hello"] = "world"
	err = r.client.Update(context.TODO(), rs)
	if err != nil {
		log.Error(err, "Could not write ReplicaSet")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// podAnnotator annotates Pods
type podAnnotator struct {
	client  client.Client
	decoder admission.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = &podAnnotator{}

// podAnnotator adds an annotation to every incoming pods.
func (a *podAnnotator) Handle(_ context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	err := a.decoder.Decode(req, pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	copy := pod.DeepCopy()

	err = mutatePodsFn(copy)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.PatchResponse(pod, copy)
}

// mutatePodsFn add an annotation to the given pod
func mutatePodsFn(pod *corev1.Pod) error {
	anno := pod.GetAnnotations()
	anno["example-mutating-admission-webhhok"] = "foo"
	pod.SetAnnotations(anno)
	return nil
}

// podValidator validates Pods
type podValidator struct {
	client  client.Client
	decoder admission.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = &podValidator{}

// podValidator admits a pod iff a specific annotation exists.
func (v *podValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	err := v.decoder.Decode(req, pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	allowed, reason, err := validatePodsFn(pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

func validatePodsFn(pod *corev1.Pod) (bool, string, error) {
	anno := pod.GetAnnotations()
	key := "example-mutating-admission-webhhok"
	_, found := anno[key]
	if found {
		return found, "", nil
	} else {
		return found, fmt.Sprintf("failed to find annotation with key: %v", key), nil
	}
}
