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

// podAnnotator annotates Pods
type podAnnotator struct {
	client  client.Client
	decoder admission.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = &podAnnotator{}

// podAnnotator adds an annotation to every incoming pods.
func (a *podAnnotator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	err := a.decoder.Decode(req, pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	copy := pod.DeepCopy()

	err = mutatePodsFn(ctx, copy)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.PatchResponse(pod, copy)
}

// mutatePodsFn add an annotation to the given pod
func mutatePodsFn(ctx context.Context, pod *corev1.Pod) error {
	v, ok := ctx.Value(admission.StringKey("foo")).(string)
	if !ok {
		return fmt.Errorf("the value associated with %v is expected to be a string", "foo")
	}
	anno := pod.GetAnnotations()
	anno["example-mutating-admission-webhook"] = v
	pod.SetAnnotations(anno)
	return nil
}
