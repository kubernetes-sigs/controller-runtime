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

package source

import (
	"fmt"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source/internal"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"

	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
)

// Source is a source of events (eh.g. Create, Update, Delete operations on Kubernetes Objects, Webhook callbacks, etc)
// which should be processed by event.EventHandlers to enqueue ReconcileRequests.
//
// * Use KindSource for events originating in the cluster (eh.g. Pod Create, Pod Update, Deployment Update).
//
// * Use ChannelSource for events originating outside the cluster (eh.g. GitHub Webhook callback, Polling external urls).
type Source interface {
	Start(eventhandler.EventHandler, workqueue.RateLimitingInterface) error
}

var _ Source = ChannelSource(make(chan event.GenericEvent))

// ChannelSource is used to provide a source of events originating outside the cluster
// (eh.g. GitHub Webhook callback).  ChannelSource requires the user to wire the external
// source (eh.g. http handler) to write GenericEvents to the underlying channel.
type ChannelSource chan event.GenericEvent

// Start implements Source and should only be called by the Controller.
func (ks ChannelSource) Start(
	handler eventhandler.EventHandler,
	queue workqueue.RateLimitingInterface) error {
	return nil
}

var log = logf.KBLog.WithName("source").WithName("KindSource")

var _ Source = &KindSource{}

// KindSource is used to provide a source of events originating inside the cluster from Watches (eh.g. Pod Create)
type KindSource struct {
	// Type is the type of object to watch.  e.g. &v1.Pod{}
	Type runtime.Object

	// informers used to watch APIs
	informers informer.Informers
}

// Start is internal and should be called only by the Controller to register an EventHandler with the Informer
// to enqueue ReconcileRequests.
func (ks *KindSource) Start(handler eventhandler.EventHandler, queue workqueue.RateLimitingInterface) error {
	// Type should have been specified by the user.
	if ks.Type == nil {
		return fmt.Errorf("must specify KindSource.Type")
	}

	// informers should have been injected before Start was called
	if ks.informers == nil {
		return fmt.Errorf("must call InjectInformers on KindSource before calling Start")
	}

	// Lookup the Informer from the Informers and add an EventHandler which populates the Queue
	i, err := ks.informers.InformerFor(ks.Type)
	if err != nil {
		return err
	}
	i.AddEventHandler(internal.EventHandler{Queue: queue, EventHandler: handler})
	return nil
}

// InjectInformers is internal should be called only by the Controller.  InjectInformers should be called before Start.
func (ks *KindSource) InjectInformers(i informer.Informers) {
	if ks.informers == nil {
		ks.informers = i
	}
}
