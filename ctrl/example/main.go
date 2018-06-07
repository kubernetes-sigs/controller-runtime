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
	"log"

	"github.com/kubernetes-sigs/kubebuilder/pkg/client"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	"github.com/kubernetes-sigs/kubebuilder/pkg/signals"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

func main() {
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(false))

	// Setup a ControllerManager
	manager, err := ctrl.NewControllerManager(ctrl.ControllerManagerArgs{})
	if err != nil {
		log.Fatal(err)
	}

	// Setup a new controller to Reconcile ReplicaSets
	c, err := manager.NewController(
		ctrl.ControllerArgs{Name: "foo-controller", MaxConcurrentReconciles: 1},
		&ReconcileReplicaSet{client: manager.GetClient()},
	)
	if err != nil {
		log.Fatal(err)
	}

	err = c.Watch(
		// Watch ReplicaSets
		&source.KindSource{Type: &appsv1.ReplicaSet{}},
		// Enqueue ReplicaSet object key
		&eventhandler.EnqueueHandler{})
	if err != nil {
		log.Fatal(err)
	}

	err = c.Watch(
		// Watch Pods
		&source.KindSource{Type: &corev1.Pod{}},
		// Enqueue Owning ReplicaSet object key
		&eventhandler.EnqueueOwnerHandler{OwnerType: &appsv1.ReplicaSet{}, IsController: true})
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(manager.Start(signals.SetupSignalHandler()))
}

// ReconcileReplicaSet reconciles ReplicaSets
type ReconcileReplicaSet struct {
	client client.Interface
}

// Implement reconcile.reconcile so the controller can reconcile objects
var _ reconcile.Reconcile = &ReconcileReplicaSet{}

func (r *ReconcileReplicaSet) Reconcile(request reconcile.ReconcileRequest) (reconcile.ReconcileResult, error) {
	// Fetch the ReplicaSet from the cache
	rs := &appsv1.ReplicaSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, rs)
	if errors.IsNotFound(err) {
		log.Printf("Could not find ReplicaSet %v.\n", request)
		return reconcile.ReconcileResult{}, nil
	}

	if err != nil {
		log.Printf("Could not fetch ReplicaSet %v for %+v\n", err, request)
		return reconcile.ReconcileResult{}, err
	}

	// Print the ReplicaSet
	log.Printf("ReplicaSet Name %s Namespace %s, Pod Name: %s\n",
		rs.Name, rs.Namespace, rs.Spec.Template.Spec.Containers[0].Name)

	// Set the label if it is missing
	if rs.Labels == nil {
		rs.Labels = map[string]string{}
	}
	if rs.Labels["hello"] == "world" {
		return reconcile.ReconcileResult{}, nil
	}

	// Update the ReplicaSet
	rs.Labels["hello"] = "world"
	err = r.client.Update(context.TODO(), rs)
	if err != nil {
		log.Printf("Could not write ReplicaSet %v\n", err)
		return reconcile.ReconcileResult{}, err
	}

	return reconcile.ReconcileResult{}, nil
}
