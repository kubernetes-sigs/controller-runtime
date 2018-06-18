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
Package controller provides types and functions for building Controllers.

Creation

To create a new Controller, first create a manager.Manager and provide it to New.  The Manager will
take care of Starting / Stopping the Controller, as well as provide shared Caches and Clients to the
Sources, EventHandlers, Predicates, and Reconcile.
*/
package controller
