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

// Package application provides high-level porcelain wrapping the controller and manager libraries for
// building Kubernetes APIs for simple applications (operators).
//
// Note: Application is an alpha library and make have backwards compatibility breaking changes.
//
// Projects built with an application can trivially be rebased on top of the underlying Controller and Manager
// packages if the project requires more customized behavior in the future.
//
// Application
//
// An application is a Controller that implements the operational logic for an application.  It is often used
// to take off-the-shelf OSS applications, and make them Kubernetes native.
package application
