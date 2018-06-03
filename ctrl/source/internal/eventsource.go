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

package internal

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var _ cache.ResourceEventHandler = EventHandler{}

// EventHandler adapts a eventhandler.EventHandler interface to a cache.ResourceEventHandler interface
type EventHandler struct {
	EH eventhandler.EventHandler
	Q  workqueue.RateLimitingInterface
}

func (e EventHandler) OnAdd(obj interface{}) {
	c := event.CreateEvent{}

	// Pull the Meta out of the object
	if o, ok := obj.(metav1.ObjectMetaAccessor); ok {
		c.Meta = o.GetObjectMeta()
	}

	// Pull the runtime.Type out of the object
	if o, ok := obj.(runtime.Object); ok {
		c.Object = o
	}

	// Invoke create handler
	e.EH.Create(e.Q, c)
}

func (e EventHandler) OnUpdate(oldObj, newObj interface{}) {
	u := event.UpdateEvent{}

	// Pull the Meta out of the object
	if o, ok := oldObj.(metav1.ObjectMetaAccessor); ok {
		u.MetaOld = o.GetObjectMeta()
	}
	// Pull the runtime.Type out of the object
	if o, ok := oldObj.(runtime.Object); ok {
		u.ObjectOld = o
	}

	// Pull the Meta out of the object
	if o, ok := newObj.(metav1.ObjectMetaAccessor); ok {
		u.MetaNew = o.GetObjectMeta()
	}
	// Pull the runtime.Type out of the object
	if o, ok := newObj.(runtime.Object); ok {
		u.ObjectNew = o
	}

	// Invoke update handler
	e.EH.Update(e.Q, u)
}

func (e EventHandler) OnDelete(obj interface{}) {
	c := event.DeleteEvent{}

	// Deal with tombstone events by pulling the object out.  Tombstone events wrap the object in a
	// DeleteFinalStateUnknown struct, so the object needs to be pulled out.
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		// If the object doesn't have Metadata, assume it is a tombstone object of type DeletedFinalStateUnknown
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}

		// Pull the Meta out of the object
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}

		// Set obj to the tombstone obj
		obj = tombstone.Obj
		glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}

	if o, ok := obj.(metav1.ObjectMetaAccessor); ok {
		c.Meta = o.GetObjectMeta()
	}

	if o, ok := obj.(runtime.Object); ok {
		c.Object = o
	}

	// Invoke delete handler
	e.EH.Delete(e.Q, c)
}
