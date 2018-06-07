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

	"github.com/kubernetes-sigs/kubebuilder/pkg/client"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/predicate"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	logf "github.com/kubernetes-sigs/kubebuilder/pkg/log"
)

var log = logf.KBLog.WithName("controller").WithName("controller")

type ControllerArgs struct {
	// name is used to uniquely identify a controller in tracing, logging and monitoring.  name is required.
	Name string

	// maxConcurrentReconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 1.
	MaxConcurrentReconciles int
}

type Controller interface {
	// Watch takes events provided by a Source and uses the EventHandler to enqueue ReconcileRequests in
	// response to the events.
	//
	// Watch may be provided one or more Predicates to filter events before they are given to the EventHandler.
	// Events will be passed to the EventHandler iff all provided Predicates evaluate to true.
	Watch(src source.Source, evthdler eventhandler.EventHandler, prct ...predicate.Predicate) error

	// Start starts the controller.  Start blocks until stop is closed or a controller has an error starting.
	Start(stop <-chan struct{}) error
}

var _ Controller = &controller{}

// Controllers are work queues that watch for changes to objects (i.e. Create / Update / Delete events) and
// then reconcile an object (i.e. make changes to ensure the system state matches what is specified in the object).
type controller struct {
	// name is used to uniquely identify a controller in tracing, logging and monitoring.  name is required.
	name string

	// maxConcurrentReconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 1.
	maxConcurrentReconciles int

	// reconcile is a function that can be called at any time with the name / Namespace of an object and
	// ensures that the state of the system matches the state specified in the object.
	// Defaults to the DefaultReconcileFunc.
	reconcile reconcile.Reconcile

	// client is a lazily initialized client.  The controllerManager will initialize this when Start is called.
	client client.Interface

	// scheme is injected by the controllerManager when controllerManager.Start is called
	scheme *runtime.Scheme

	// fieldIndexes knows how to add field indexes over the Informers used by this controller,
	// which can later be consumed via field selectors from the injected client.
	fieldIndexes client.FieldIndexer

	// Informers are injected by the controllerManager when controllerManager.Start is called
	informers informer.Informers

	// config is the rest.config used to talk to the apiserver.  Defaults to one of in-cluster, environment variable
	// specified, or the ~/.kube/config.
	config *rest.Config

	// queue is an listeningQueue that listens for events from Informers and adds object keys to
	// the queue for processing
	queue workqueue.RateLimitingInterface

	// once ensures unspecified fields get default values
	once sync.Once

	inject func(i interface{})

	// TODO(pwittrock): Consider initializing a logger with the controller name as the tag
}

func (c *controller) Watch(src source.Source, evthdler eventhandler.EventHandler, prct ...predicate.Predicate) error {
	// Inject cache into arguments
	c.inject(src)
	c.inject(evthdler)
	for _, pr := range prct {
		c.inject(pr)
	}

	// TODO(pwittrock): wire in predicates

	log.Info("Starting EventSource", "controller", c.name, "Source", src)
	return src.Start(evthdler, c.queue)
}

func (c *controller) Start(stop <-chan struct{}) error {
	// TODO)(pwittrock): Reconsider HandleCrash
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	// Start the SharedIndexInformer factories to begin populating the SharedIndexInformer caches
	log.Info("Starting controller", "controller", c.name)

	// Wait for the caches to be synced before starting workers
	allInformers := c.informers.KnownInformersByType()
	syncedFuncs := make([]cache.InformerSynced, 0, len(allInformers))
	for _, informer := range allInformers {
		syncedFuncs = append(syncedFuncs, informer.HasSynced)
	}
	if ok := cache.WaitForCacheSync(stop, syncedFuncs...); !ok {
		err := fmt.Errorf("failed to wait for %s caches to sync", c.name)
		log.Error(err, "Could not wait for Cache to sync", "controller", c.name)
		return err
	}

	// Launch two workers to process resources
	log.Info("Starting workers", "controller", c.name, "WorkerCount", c.maxConcurrentReconciles)
	for i := 0; i < c.maxConcurrentReconciles; i++ {
		// Continually process work items
		go wait.Until(func() {
			// TODO(pwittrock): Should we really use wait.Until to continuously restart this if it exits?
			for c.processNextWorkItem() {
			}
		}, time.Second, stop)
	}

	<-stop
	log.Info("Stopping workers", "controller", c.name)
	return nil
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *controller) processNextWorkItem() bool {
	// This code copy-pasted from the sample-controller.

	obj, shutdown := c.queue.Get()
	if obj == nil {
		log.Error(nil, "Encountered nil ReconcileRequest", "Object", obj)
		c.queue.Forget(obj)
	}

	if shutdown {
		// Return false, take a break before starting again.  But Y tho?
		return false
	}

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
		log.Error(nil, "Queue item was not a ReconcileRequest",
			"controller", c.name, "Type", fmt.Sprintf("%T", obj), "Value", obj)
		// Return true, don't take a break
		return true
	}

	// RunInformersAndControllers the syncHandler, passing it the namespace/name string of the
	// resource to be synced.
	if result, err := c.reconcile.Reconcile(req); err != nil {
		c.queue.AddRateLimited(req)
		log.Error(nil, "reconcile error", "controller", c.name, "ReconcileRequest", req)

		// TODO(pwittrock): FTO Returning an error here seems to back things off for a second before restarting
		// the loop through wait.Util.
		// Return false, take a break. But y tho?
		return false
	} else if result.Requeue {
		c.queue.AddRateLimited(req)
		// Return true, don't take a break
		return true
	}

	// Finally, if no error occurs we Forget this item so it does not
	// get queued again until another change happens.
	c.queue.Forget(obj)

	// TODO(directxman12): What does 1 mean?  Do we want level constants?  Do we want levels at all?
	log.V(1).Info("Successfully Reconciled", "controller", c.name, "ReconcileRequest", req)

	// Return true, don't take a break
	return true
}
