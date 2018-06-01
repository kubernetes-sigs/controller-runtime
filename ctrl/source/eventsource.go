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

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/informer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Source is a source of events (e.g. Create, Update, Delete operations on Kubernetes Objects, Webhook callbacks, etc)
// which should be processed by event.EventHandlers to enqueue ReconcileRequests.
//
// * Use KindSource for events originating in the cluster (e.g. Pod Create, Pod Update, Deployment Update).
//
// * Use ChannelSource for events originating outside the cluster (e.g. GitHub Webhook callback, Polling external urls).
type Source interface {
	Start(eventhandler.EventHandler, workqueue.RateLimitingInterface) error
}

// Config provides shared structures required for starting a Source.
type Config struct{}

var _ Source = ChannelSource(make(chan event.GenericEvent))

// ChannelSource is used to provide a source of events originating outside the cluster
// (e.g. GitHub Webhook callback).  ChannelSource requires the user to wire the external
// source (e.g. http handler) to write GenericEvents to the underlying channel.
type ChannelSource chan event.GenericEvent

// Start implements Source and should only be called by the Controller.
func (ks ChannelSource) Start(
	handler eventhandler.EventHandler,
	queue workqueue.RateLimitingInterface) error {

	return nil
}

var _ Source = KindSource{}

// KindSource is used to provide a source of events originating inside the cluster from Watches (e.g. Pod Create)
type KindSource struct {
	Group   string
	Version string
	Kind    string

	Object runtime.Object

	informerCache informer.IndexInformerCache
}

// Start implements Source and should only be called by the Controller to start the Source watching events.
func (ks KindSource) Start(
	handler eventhandler.EventHandler,
	queue workqueue.RateLimitingInterface) error {

	// TODO: If the informerCache cache isn't set, use the default package level implementation

	i, err := ks.informerCache.GetSharedIndexInformer(schema.GroupVersionKind{
		Group:   ks.Group,
		Kind:    ks.Version,
		Version: ks.Kind,
	}, ks.Object)

	if err != nil {
		return err
	}

	i.AddEventHandler(EventHandler{
		q: queue,
		e: handler,
	})

	return nil
}

func (ks KindSource) SetInformerCache(informer informer.IndexInformerCache) {
	ks.informerCache = informer
}

var _ cache.ResourceEventHandler = EventHandler{}

type EventHandler struct {
	e eventhandler.EventHandler
	q workqueue.RateLimitingInterface
}

func (e EventHandler) OnAdd(obj interface{}) {
	c := event.CreateEvent{}
	if o, ok := obj.(metav1.ObjectMetaAccessor); ok {
		c.Meta = o.GetObjectMeta()
	}
	if o, ok := obj.(runtime.Object); ok {
		c.Object = o
	}
	e.e.Create(e.q, c)
}

func (e EventHandler) OnUpdate(oldObj, newObj interface{}) {
	u := event.UpdateEvent{}
	if o, ok := oldObj.(metav1.ObjectMetaAccessor); ok {
		u.MetaOld = o.GetObjectMeta()
	}
	if o, ok := oldObj.(runtime.Object); ok {
		u.ObjectOld = o
	}

	if o, ok := newObj.(metav1.ObjectMetaAccessor); ok {
		u.MetaNew = o.GetObjectMeta()
	}
	if o, ok := newObj.(runtime.Object); ok {
		u.ObjectNew = o
	}

	e.e.Update(e.q, u)
}

func (e EventHandler) OnDelete(obj interface{}) {
	c := event.DeleteEvent{}

	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		// Set obj to the tombstone obj
		obj = tombstone.Obj
		glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	c.Meta = object

	if o, ok := obj.(runtime.Object); ok {
		c.Object = o
	}

	e.e.Delete(e.q, c)
}
