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

/*
Package pkg provides libraries for building Controllers.  Controllers implement Kubernetes APIs
and are central to building Operators, Workload APIs, Configuration APIs, Autoscalers, and more.

Client

Client provides a Read / Write client for reading and writing objects to an apiserver.

Cache

Cache provides a Read client for reading objects from an apiserver that are stored in a local cache.
Cache supports registering handlers to respond to events that update the cache.

Manager

Manager provides a mechanism for Starting components and provides shared Caches and Clients to the components.

Controller

Controller is a work queue that enqueues work in response to source.Source events (e.g. Pod Create, Update, Delete)
and triggers reconcile.Reconcile functions when the work is dequeued.

Unlike http handlers, Controllers DO NOT perform work directly in response to events, but instead enqueue
reconcile.Requests so the work is performed eventually.

* Controllers run reconcile.Reconcile functions against objects (provided as name / Namespace).

* Controllers enqueue reconcile.Requests in response events provided by source.Sources.

Reconcile

reconcile.Reconcile is a function that may be called at anytime with the Name and Namespace of an
object.  When called, it will ensure that the state of the system matches what is specified in the object at the
time Reconcile is called.

Example: Reconcile is run against a ReplicationController object.  The ReplicationController specifies 5 replicas.
3 Pods exist in the system.  Reconcile creates 2 more Pods and sets their OwnerReference to point at the
ReplicationController.

* reconcile works on a single object type. - e.g. it will only reconcile ReplicaSets.

* reconcile is triggered by a reconcile.Request containing the name / Namespace of an object to reconcile.

* reconcile does not care about the event contents or event type triggering the reconcile.Request.
- e.g. it doesn't matter whether a ReplicaSet was created or updated, reconcile will check that the correct
Pods exist either way.

* Users MUST implement Reconcile for Controllers.

Source

resource.Source provides a stream of events.  Events may be internal events from watching Kubernetes
APIs (e.g. Pod Create, Update, Delete), or may be synthetic Generic events triggered by cron or WebHooks
(e.g. through a Slackbot or GitHub callback).

Example 1: source.Kind uses the Kubernetes API Watch endpoint for a GroupVersionKind to provide
Create, Update, Delete events.

Example 2: source.Channel reads Generic events from a channel fed by a WebHook called from a Slackbot.

* source provides a stream of events for EventHandlers to handle.

* source may provide either events from Watches (e.g. object Create, Update, Delete) or Generic triggered
from another source (e.g. WebHook callback).

* Users SHOULD use the provided Source implementations instead of implementing their own for nearly all cases.

EventHandler

eventhandler.EventHandler maps from a source.Source into reconcile.Requests which are enqueued as work for the
Controller.

Example: a Pod Create event from a Source is provided to the eventhandler.EnqueueHandler, which enqueues a
reconcile.Request containing the name / Namespace of the Pod.

* EventHandler takes an event.Event and enqueues reconcile.Requests

* EventHandlers MAY map an event for an object of one type to a reconcile.Request for an object of another type.

* EventHandlers MAY map an event for an object to multiple reconcile.Requests for different objects.

* Users SHOULD use the provided EventHandler implementations instead of implementing their own for almost all cases.

Predicate

predicate.Predicate allows events to be filtered before they are given to EventHandlers.  This allows common
filters to be reused and composed together with EventHandlers.

* Predicate takes and event.Event and returns a bool (true to enqueue)

* Predicates are optional

* Users SHOULD use the provided Predicate implementations, but MAY implement their own Predicates as needed.

PodController Diagram

Source provides event:

* &source.KindSource{"core", "v1", "Pod"} -> (Pod foo/bar Create Event)

EventHandler enqueues Request:

* &eventhandler.Enqueue{} -> (reconcile.Request{"foo", "bar"})

Reconcile is called with the Request:

* Reconcile(reconcile.Request{"foo", "bar"})

Usage

The following example shows creating a new Controller program which Reconciles ReplicaSet objects in response
to Pod or ReplicaSet events.  The Reconcile function simply adds a label to the ReplicaSet.

See the example/main.go for a usage example.

Controller Example

1. Watch ReplicaSet and Pods Sources

1.1 ReplicaSet -> eventhandler.EnqueueHandler - enqueue the ReplicaSet Namespace and Name.

1.2 Pod (created by ReplicaSet) -> eventhandler.EnqueueOwnerHandler - enqueue the Owning ReplicaSet key.

2. Reconcile ReplicaSet

2.1 ReplicaSet object created -> Read ReplicaSet, try to read Pods -> if is missing create Pods.

2.2 reconcile triggered by creation of Pods -> Read ReplicaSet and Pods, do nothing.

2.3 reconcile triggered by deletion of Pods -> Read ReplicaSet and Pods, create replacement Pods.

Watching and EventHandling

Controllers may Watch multiple Kinds of objects (e.g. Pods, ReplicaSets and Deployments), but they should
enqueue keys for only a single Type.  When one Type of object must be be updated in response to changes
in another Type of object, an EnqueueMappedHandler may be used to reconcile the Type that is being
updated and watch the other Type for Events.  e.g. Respond to a cluster resize
Event (add / delete Node) by re-reconciling all instances of another Type that cares about the cluster size.

For example, a Deployment controller might use an EnqueueHandler and EnqueueOwnerHandler to:

* Watch for Deployment Events - enqueue the key of the Deployment.

* Watch for ReplicaSet Events - enqueue the key of the Deployment that created the ReplicaSet (e.g the Owner)

Note: reconcile.Requests are deduplicated when they are enqueued.  Many Pod Events for the same ReplicaSet
may trigger only 1 reconcile invocation as each Event results in the Handler trying to enqueue
the same reconcile.Request for the ReplicaSet.

Controller Writing Tips

Reconcile Runtime Complexity:

* It is better to write Controllers to perform an O(1) reconcile N times (e.g. on N different objects) instead of
performing an O(N) reconcile 1 time (e.g. on a single object which manages N other objects).

* Example: If you need to update all Services in response to a Node being added - reconcile Services but Watch
Node events (transformed to Service object name / Namespaces) instead of Reconciling the Node and updating all
Services from a single reconcile.

Event Multiplexing:

* reconcile.Requests for the same name / Namespace are deduplicated when they are enqueued.  This allows
for Controllers to gracefully handle event storms for a single object.  Multiplexing multiple event Sources to
a single object type takes advantage of this.

* Example: Pod events for a ReplicaSet are transformed to a ReplicaSet name / Namespace, so the ReplicaSet
will be Reconciled only 1 time for multiple Pods.
*/
package pkg
