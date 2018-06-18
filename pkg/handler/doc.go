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
Package handler defines EventHandlers that enqueue reconcile.Requests in response to Create, Update, Deletion Events
observed from Watching Kubernetes APIs.

EventHandlers

Enqueue - Enqueues a reconcile.Request containing the Name and Namespace of the object in the Event.

EnqueueOwner - Enqueues a reconcile.Request containing the Name and Namespace of the Owner of the object in the Event.

EnqueueMapped - Enqueues Reconcile.Requests resulting from a user provided transformation function run against the
object in the Event.
*/
package handler
