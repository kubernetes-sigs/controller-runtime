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

package main

import (
	"context"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// podValidator validates Pods
type podValidator struct {
	client  client.Client
	decoder admission.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = &podValidator{}

// podValidator admits a pod iff a specific annotation exists.
func (v *podValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	err := v.decoder.Decode(req, pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	allowed, reason, err := validatePodsFn(ctx, pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

func validatePodsFn(ctx context.Context, pod *corev1.Pod) (bool, string, error) {
	v, ok := ctx.Value(admission.StringKey("foo")).(string)
	if !ok {
		return false, "",
			fmt.Errorf("the value associated with key %q is expected to be a string", v)
	}
	annotations := pod.GetAnnotations()
	key := "example-mutating-admission-webhook"
	anno, found := annotations[key]
	switch {
	case !found:
		return found, fmt.Sprintf("failed to find annotation with key: %q", key), nil
	case found && anno == v:
		return found, "", nil
	case found && anno != v:
		return false,
			fmt.Sprintf("the value associate with key %q is expected to be %q, but got %q", "foo", v, anno), nil
	}
	return false, "", nil
}
