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

package ctrl_test

import (
	"log"

	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/predicate"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// This example creates a new controller named "pod-controller" with a no-op reconcile function and registers
// it with the DefaultControllerManager.
func ExampleController() {
	cm, err := ctrl.NewControllerManager(ctrl.ControllerManagerArgs{Config: config.GetConfigOrDie()})
	if err != nil {
		log.Fatal(err)
	}
	_, err = cm.NewController(
		ctrl.ControllerArgs{Name: "pod-controller", MaxConcurrentReconciles: 1},
		reconcile.ReconcileFunc(func(o reconcile.ReconcileRequest) (reconcile.ReconcileResult, error) {
			// Your business logic to implement the API by creating, updating, deleting objects goes here.
			return reconcile.ReconcileResult{}, nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
}

// This example watches Pods and enqueues ReconcileRequests with the changed Pod Name and Namespace.
func ExampleController_Watch_1() {
	cm, err := ctrl.NewControllerManager(ctrl.ControllerManagerArgs{Config: config.GetConfigOrDie()})
	if err != nil {
		log.Fatal(err)
	}
	c, err := cm.NewController(ctrl.ControllerArgs{Name: "foo-controller"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	err = c.Watch(&source.KindSource{Type: &corev1.Pod{}}, &eventhandler.EnqueueHandler{})
	if err != nil {
		log.Fatal(err)
	}
}

// This example watches Deployments and enqueues ReconcileRequests with the change Deployment Name and Namespace
// iff 1. the Event is not Update or 2. the Generation of the Deployment object changed in the Update.
func ExampleController_Watch_2() {
	cm, err := ctrl.NewControllerManager(ctrl.ControllerManagerArgs{Config: config.GetConfigOrDie()})
	if err != nil {
		log.Fatal(err)
	}
	c, err := cm.NewController(ctrl.ControllerArgs{Name: "foo-controller"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	err = c.Watch(&source.KindSource{Type: &appsv1.Deployment{}}, &eventhandler.EnqueueHandler{},
		predicate.PredicateFuncs{UpdateFunc: func(e event.UpdateEvent) bool {
			return e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration()
		}},
	)
	if err != nil {
		log.Fatal(err)
	}
}
