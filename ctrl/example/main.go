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
	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/inject"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	"github.com/kubernetes-sigs/kubebuilder/pkg/signals"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	flag.Parse()

	// Create the ControllerManager and Controller
	cm := ctrl.ControllerManager{Config: config.GetConfigOrDie()}
	c := &ctrl.Controller{Reconcile: &Reconcile{}}

	// Watch Pods and ReplicaSets
	cm.AddController(c, func() {
		c.Watch(
			&source.KindSource{Type: &appsv1.ReplicaSet{}},
			&eventhandler.EnqueueHandler{})
		c.Watch(
			&source.KindSource{Type: &corev1.Pod{}},
			&eventhandler.EnqueueOwnerHandler{OwnerType: &appsv1.ReplicaSet{}, IsController: true})
	})

	// Start the Controllers and block
	cm.Start(signals.SetupSignalHandler())
}

var _ inject.Client = &Reconcile{}
var _ reconcile.Reconcile = &Reconcile{}

type Reconcile struct {
	client client.Interface
}

// InjectClient is used by the Controller to inject a client.Interface
func (r *Reconcile) InjectClient(c client.Interface) {
	r.client = c
}

func (r *Reconcile) Reconcile(request reconcile.ReconcileRequest) (reconcile.ReconcileResult, error) {
	log.Printf("ReconcileRequest: %+v\n", request)
	rs := &appsv1.ReplicaSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, rs)
	if err != nil {
		return reconcile.ReconcileResult{}, err
	}

	log.Printf("ReplicaSet Pod Name: %+v\n", rs.Spec.Template.Spec.Containers[0].Name)
	return reconcile.ReconcileResult{}, nil
}
