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

package ctrl

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/predicate"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controllers are work queues that watch for changes to objects (i.e. Create / Update / Delete events) and
// then Reconcile an object (i.e. make changes to ensure the system state matches what is specified in the object).
type Controller struct {
	// Name is used to uniquely identify a Controller in tracing, logging and monitoring.  Name is required.
	Name string

	// Reconcile is a function that can be called at any time with the Name / Namespace of an object and
	// ensures that the state of the system matches the state specified in the object.
	// Defaults to the DefaultReconcileFunc.
	Reconcile reconcile.Reconcile

	// MaxConcurrentReconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 1.
	MaxConcurrentReconciles int

	// Stop is used to shutdown the Reconcile.  Defaults to a new channel.
	Stop <-chan struct{}

	// listeningQueue is an listeningQueue that listens for events from informers and adds object keys to
	// the queue for processing
	queue workqueue.RateLimitingInterface

	// synced is a slice of functions that return whether or not all informers have been synced
	synced []cache.InformerSynced

	// once ensures unspecified fields get default values
	once sync.Once
}

// Watch takes events provided by a Source and uses the EventHandler to enqueue ReconcileRequests in
// response to the events.
//
// Watch may be provided one or more Predicates to filter events before they are given to the EventHandler.
// Events will be passed to the EventHandler iff all provided Predicates evaluate to true.
func (*Controller) Watch(source.Source, eventhandler.EventHandler, ...predicate.Predicate) {

}

// init defaults field values on c
func (c *Controller) init() {
	if len(c.Name) == 0 {
		c.Name = "controller-unamed"
	}

	// Default the RateLimitingInterface to a NamedRateLimitingQueue
	if c.queue == nil {
		c.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), c.Name)
	}
}

// Start starts the Controller.  Start blocks until the Stop channel is closed.
func (c *Controller) Start() error {
	c.once.Do(c.init)
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	// Start the SharedIndexInformer factories to begin populating the SharedIndexInformer caches
	glog.Infof("Starting %s controller", c.Name)

	// Wait for the caches to be synced before starting workers
	glog.Infof("Waiting for %s SharedIndexInformer caches to sync", c.Name)
	if ok := cache.WaitForCacheSync(c.Stop, c.synced...); !ok {
		return fmt.Errorf("failed to wait for %s caches to sync", c.Name)
	}

	glog.Infof("Starting %s workers", c.Name)
	// Launch two workers to process resources
	for i := 0; i < c.MaxConcurrentReconciles; i++ {
		// Continually process work items
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, c.Stop)
	}

	glog.Infof("Started %s workers", c.Name)
	<-c.Stop
	glog.Infof("Shutting %s down workers", c.Name)

	return nil
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.queue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workque   ue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.queue.Done(obj)
		var req reconcile.ReconcileRequest
		var ok bool
		if req, ok = obj.(reconcile.ReconcileRequest); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.queue.Forget(obj)
			runtime.HandleError(fmt.Errorf(
				"expected reconcile.ReconcileRequest in %s workqueue but got %#v", c.Name, obj))
			return nil
		}

		// RunInformersAndControllers the syncHandler, passing it the namespace/Name string of the
		// resource to be synced.
		if result, err := c.Reconcile.Reconcile(req); err != nil {
			c.queue.AddRateLimited(req)
			return fmt.Errorf("error syncing %s queue '%+v': %s", c.Name, req, err.Error())
		} else if result.Requeue {
			c.queue.AddRateLimited(req)
		}

		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.queue.Forget(obj)
		glog.Infof("Successfully synced %s queue '%+v'", c.Name, req)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}
