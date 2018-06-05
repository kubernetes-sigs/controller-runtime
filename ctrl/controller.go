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

	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/inject"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/predicate"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
)

var log = logf.KBLog.WithName("controller").WithName("Controller")

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

	// informers is the IndexInformerCache
	informers informer.IndexInformerCache

	// config is the rest.config used to talk to the apiserver.  Defaults to one of in-cluster, environment variable
	// specified, or the ~/.kube/config.
	config *rest.Config

	// queue is an listeningQueue that listens for events from informers and adds object keys to
	// the queue for processing
	queue workqueue.RateLimitingInterface

	// synced is a slice of functions that return whether or not all informers have been synced
	synced []cache.InformerSynced

	start []func()

	// once ensures unspecified fields get default values
	once sync.Once

	startInformers bool
}

func (c *Controller) InjectIndexInformerCache(i informer.IndexInformerCache) {
	c.informers = i
}

func (c *Controller) InjectConfig(i *rest.Config) {
	c.config = i
}

// Watch takes events provided by a Source and uses the EventHandler to enqueue ReconcileRequests in
// response to the events.
//
// Watch may be provided one or more Predicates to filter events before they are given to the EventHandler.
// Events will be passed to the EventHandler iff all provided Predicates evaluate to true.
func (c *Controller) Watch(s source.Source, e eventhandler.EventHandler, p ...predicate.Predicate) {

	// Setup the watch to run when the controller is started
	c.start = append(c.start, func() {
		// Inject cache into arguments
		inject.InjectConfig(c.config, s)
		inject.InjectIndexInformerCache(c.informers, s)

		inject.InjectConfig(c.config, e)
		inject.InjectIndexInformerCache(c.informers, e)

		for _, pr := range p {
			inject.InjectIndexInformerCache(c.informers, pr)
			inject.InjectConfig(c.config, pr)
		}

		log.Info("Starting EventSource", "Controller", c.Name, "Source", s)
		s.Start(e, c.queue)
	})
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

	if c.informers == nil {
		log.Info("Creating new Informers", "Controller", c.Name)
		c.startInformers = true
		if c.config == nil {
			c.config, _ = config.GetConfig()
		}

		c.informers = &informer.IndexedCache{
			Config: c.config,
		}
	}
}

// Start starts the Controller.  Start returns once the Controller has started.
func (c *Controller) Start(stop <-chan struct{}) (chan<- error, error) {
	done := make(chan<- error)
	c.once.Do(c.init)

	// Start each of the watches.  This must happen before starting the Informers.
	for _, f := range c.start {
		f()
	}

	// Start the informers if we are managing them
	if c.startInformers {
		log.Info("Starting Informers from Controller", "Controller", c.Name)
		c.informers.Start(stop)
	}

	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	// Start the SharedIndexInformer factories to begin populating the SharedIndexInformer caches
	log.Info("Starting Controller", "Controller", c.Name)

	// TODO: Figure out how to wait for caches to sync for the dynamic cache

	// Wait for the caches to be synced before starting workers
	if ok := cache.WaitForCacheSync(stop, c.synced...); !ok {
		err := fmt.Errorf("failed to wait for %s caches to sync", c.Name)
		log.Error(err, "Could not wait for Cache to sync", "Controller", c.Name)
		return nil, err
	}

	// Launch two workers to process resources
	log.Info("Starting workers", "Controller", c.Name, "WorkerCount", c.MaxConcurrentReconciles)
	for i := 0; i < c.MaxConcurrentReconciles; i++ {
		// Continually process work items
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, stop)
	}

	go func() {
		<-stop
		log.Info("Stopping workers", "Controller", c.Name)
		close(done)
	}()
	return done, nil
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
			err := fmt.Errorf(
				"expected reconcile.ReconcileRequest in %s workqueue but got %#v", c.Name, obj)
			runtime.HandleError(err)
			log.Error(err, "Non ReconcileRequest in queue", "Controller", c.Name, "Value", obj)
			return nil
		}

		// RunInformersAndControllers the syncHandler, passing it the namespace/Name string of the
		// resource to be synced.
		if result, err := c.Reconcile.Reconcile(req); err != nil {
			c.queue.AddRateLimited(req)
			err := fmt.Errorf("error syncing %s queue '%+v': %s", c.Name, req, err.Error())
			log.Error(err, "Reconcile error", "Controller", c.Name, "ReconcileRequest", req)

			return err
		} else if result.Requeue {
			c.queue.AddRateLimited(req)
		}

		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.queue.Forget(obj)
		log.Info("Successfully Reconciled", "Controller", c.Name, "ReconcileRequest", req)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}
