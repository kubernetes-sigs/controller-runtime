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

package controller_test

import (
	"log"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client/config"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/eventhandler"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/reconcile"
	"github.com/kubernetes-sigs/controller-runtime/pkg/controller/source"
	corev1 "k8s.io/api/core/v1"
)

// This example creates a new controller named "pod-controller" with a no-op reconcile function and registers
// it with the DefaultControllerManager.
func ExampleController() {
	cm, err := controller.NewManager(controller.ManagerArgs{Config: config.GetConfigOrDie()})
	if err != nil {
		log.Fatal(err)
	}
	_, err = cm.NewController(
		controller.Options{Name: "pod-controller", MaxConcurrentReconciles: 1},
		reconcile.Func(func(o reconcile.Request) (reconcile.Result, error) {
			// Your business logic to implement the API by creating, updating, deleting objects goes here.
			return reconcile.Result{}, nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
}

// This example watches Pods and enqueues reconcile.Requests with the changed Pod Name and Namespace.
func ExampleController_Watch() {
	cm, err := controller.NewManager(controller.ManagerArgs{Config: config.GetConfigOrDie()})
	if err != nil {
		log.Fatal(err)
	}
	c, err := cm.NewController(controller.Options{Name: "foo-controller"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	err = c.Watch(&source.KindSource{Type: &corev1.Pod{}}, &eventhandler.EnqueueHandler{})
	if err != nil {
		log.Fatal(err)
	}
}
