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

package source_test

import (
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var ctrl controller.Controller
var mgr manager.Manager

// This example Watches for Pod Events (e.g. Create / Update / Delete) and enqueues a reconcile.Request
// with the Name and Namespace of the Pod.
func ExampleKind() {
	ctrl.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{})
}

// This example reads GenericEvents from a channel and enqueues a reconcile.Request containing the Name and Namespace
// provided by the event.
func ExampleChannel() {
	events := make(chan event.GenericEvent)

	ctrl.Watch(
		&source.Channel{Source: events},
		&handler.EnqueueRequestForObject{},
	)
}

// This example Watches for Service Events (e.g. Create / Update / Delete) and enqueues a reconcile.Request
// with the Name and Namespace of the Service.  It uses the client-go generated Service Informer instead of the
// Generic Informer.
func ExampleInformer() {
	generatedClient := kubernetes.NewForConfigOrDie(mgr.GetConfig())
	generatedInformers := kubeinformers.NewSharedInformerFactory(generatedClient, time.Minute*30)

	// Add it to the Manager
	if err := mgr.Add(manager.StartAdapter(generatedInformers.Start)); err != nil {
		glog.Fatalf("error Adding InformerFactory to the Manager: %v", err)
	}

	// Setup Watch using the client-go generated Informer
	if err := ctrl.Watch(
		&source.Informer{InformerProvider: generatedInformers.Core().V1().Services()},
		&handler.EnqueueRequestForObject{},
	); err != nil {
		glog.Fatalf("error Watching Services: %v", err)
	}

	// Start the Manager
	mgr.Start(signals.SetupSignalHandler())
}
