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

package webhook_test

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	. "sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	mgr ctrl.Manager
)

func Example() {
	// Build webhooks
	// These handlers could be also be implementations
	// of the AdmissionHandler interface for more complex
	// implementations.
	mutatingHook := &Admission{
		Handler: admission.HandlerFunc(func(ctx context.Context, req AdmissionRequest) AdmissionResponse {
			return Patched("some changes",
				JSONPatchOp{Operation: "add", Path: "/metadata/annotations/access", Value: "granted"},
				JSONPatchOp{Operation: "add", Path: "/metadata/annotations/reason", Value: "not so secret"},
			)
		}),
	}

	validatingHook := &Admission{
		Handler: admission.HandlerFunc(func(ctx context.Context, req AdmissionRequest) AdmissionResponse {
			return Denied("none shall pass!")
		}),
	}

	// Create a webhook server.
	hookServer := &Server{
		Port: 8443,
	}
	if err := mgr.Add(hookServer); err != nil {
		panic(err)
	}

	// Register the webhooks in the server.
	hookServer.Register("/mutating", mutatingHook)
	hookServer.Register("/validating", validatingHook)

	// Start the server by starting a previously-set-up manager
	err := mgr.Start(ctrl.SetupSignalHandler())
	if err != nil {
		// handle error
		panic(err)
	}
}
