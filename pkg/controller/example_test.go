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

	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var mrg manager.Manager

// This example creates a new Controller named "pod-controller" with a no-op reconcile function.  The
// manager.Manager will be used to Start the Controller, and will provide it a shared Cache and Client.
func ExampleNew() {
	_, err := controller.New("pod-controller", mrg, controller.Options{
		Reconcile: reconcile.Func(func(o reconcile.Request) (reconcile.Result, error) {
			// Your business logic to implement the API by creating, updating, deleting objects goes here.
			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		log.Fatal(err)
	}
}

// This example creates a new Controller named "pod-controller" with a no-op reconcile function and registers
// it with the DefaultControllerManager.
func ExampleController() {
	_, err := controller.New("pod-controller", mrg, controller.Options{
		Reconcile: reconcile.Func(func(o reconcile.Request) (reconcile.Result, error) {
			// Your business logic to implement the API by creating, updating, deleting objects goes here.
			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		log.Fatal(err)
	}
	mrg.Start(signals.SetupSignalHandler())
}

// This example watches Pods and enqueues reconcile.Requests with the changed Pod Name and Namespace.
func ExampleController_Watch() {
	c, err := controller.New("pod-controller", mrg, controller.Options{
		Reconcile: reconcile.Func(func(o reconcile.Request) (reconcile.Result, error) {
			// Your business logic to implement the API by creating, updating, deleting objects goes here.
			return reconcile.Result{}, nil
		}),
	})
	if err != nil {
		log.Fatal(err)
	}

	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.Enqueue{})
	if err != nil {
		log.Fatal(err)
	}

	mrg.Start(signals.SetupSignalHandler())

}
