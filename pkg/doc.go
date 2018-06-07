/*
Copyright 2017 The Kubernetes Authors.

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

// The pkg package is a directory containing other packages
//
// Controller
//
// The controller package contains libraries for implementing Kubernetes APIs as Controllers by watching
// resources in a Kubernetes apiserver.
//
// Client
//
// The client package contains libraries for performing CRUD operations against a Kubernetes apiserver.
// Typically a client should be create by a ControllerManager from the controller package.
//
// Runtime
//
// The runtime package contains convenience utilities for logging and signal handling.
package pkg
