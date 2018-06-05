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

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var log = logf.KBLog.WithName("source").WithName("EventHandler")

var _ cache.ResourceEventHandler = EventHandler{}

// EventHandler adapts a eventhandler.EventHandler interface to a cache.ResourceEventHandler interface
type EventHandler struct {
	EventHandler eventhandler.EventHandler
	Queue        workqueue.RateLimitingInterface
}

func (e EventHandler) OnAdd(obj interface{}) {
	c := event.CreateEvent{}

	// Pull metav1.Object out of the object
	if o, err := meta.Accessor(obj); err == nil {
		c.Meta = o
	} else {
		log.Error(err, "OnAdd missing Meta",
			"Object", obj, "Type", fmt.Sprintf("%T", obj))
		return
	}

	// Pull the runtime.Object out of the object
	if o, ok := obj.(runtime.Object); ok {
		c.Object = o
	} else {
		log.Error(nil, "OnAdd missing runtime.Object",
			"Object", obj, "Type", fmt.Sprintf("%T", obj))
		return
	}

	// Invoke create handler
	e.EventHandler.Create(e.Queue, c)
}

func (e EventHandler) OnUpdate(oldObj, newObj interface{}) {
	u := event.UpdateEvent{}

	// Pull metav1.Object out of the object
	if o, err := meta.Accessor(oldObj); err == nil {
		u.MetaOld = o
	} else {
		log.Error(err, "OnUpdate missing MetaOld",
			"Object", oldObj, "Type", fmt.Sprintf("%T", oldObj))
		return
	}

	// Pull the runtime.Object out of the object
	if o, ok := oldObj.(runtime.Object); ok {
		u.ObjectOld = o
	} else {
		log.Error(nil, "OnUpdate missing ObjectOld",
			"Object", oldObj, "Type", fmt.Sprintf("%T", oldObj))
		return
	}

	// Pull metav1.Object out of the object
	if o, err := meta.Accessor(newObj); err == nil {
		u.MetaNew = o
	} else {
		log.Error(err, "OnUpdate missing MetaNew",
			"Object", newObj, "Type", fmt.Sprintf("%T", newObj))
		return
	}

	// Pull the runtime.Object out of the object
	// TODO: Add logging for nil stuff here
	if o, ok := newObj.(runtime.Object); ok {
		u.ObjectNew = o
	} else {
		log.Error(nil, "OnUpdate missing ObjectNew",
			"Object", oldObj, "Type", fmt.Sprintf("%T", oldObj))
		return
	}

	// Invoke update handler
	e.EventHandler.Update(e.Queue, u)
}

func (e EventHandler) OnDelete(obj interface{}) {
	c := event.DeleteEvent{}

	// Deal with tombstone events by pulling the object out.  Tombstone events wrap the object in a
	// DeleteFinalStateUnknown struct, so the object needs to be pulled out.
	// Copied from sample-controller
	// This should never happen if we aren't missing events, which we have concluded that we are not
	// and made decisions off of this belief.  Maybe this shouldn't be here?
	var ok bool
	if _, ok = obj.(metav1.Object); !ok {
		// If the object doesn't have Metadata, assume it is a tombstone object of type DeletedFinalStateUnknown
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Error(nil, "Error decoding objects.  Expected cache.DeletedFinalStateUnknown",
				"Type", fmt.Sprintf("%T", obj),
				"Object", obj)
			return
		}

		// Pull the Meta out of the object
		// TODO: Make this an Accessor
		_, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			log.Error(nil, "Error decoding object in tombstone.  Expected metav1.Object.",
				"Type", fmt.Sprintf("%T", tombstone.Obj),
				"Object", tombstone.Obj)
			return
		}

		// Set obj to the tombstone obj
		obj = tombstone.Obj
	}

	// Pull metav1.Object out of the object
	if o, err := meta.Accessor(obj); err == nil {
		c.Meta = o
	} else {
		log.Error(err, "OnAdd missing Meta",
			"Object", obj, "Type", fmt.Sprintf("%T", obj))
		return
	}

	// Pull the runtime.Object out of the object
	if o, ok := obj.(runtime.Object); ok {
		c.Object = o
	} else {
		log.Error(nil, "OnAdd missing runtime.Object",
			"Object", obj, "Type", fmt.Sprintf("%T", obj))
		return
	}

	// Invoke delete handler
	e.EventHandler.Delete(e.Queue, c)
}
