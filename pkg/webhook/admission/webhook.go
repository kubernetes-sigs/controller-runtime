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

package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/appscode/jsonpatch"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

// Handler can handle an AdmissionRequest.
type Handler interface {
	Handle(context.Context, Request) Response
}

// HandlerFunc implements Handler interface using a single function.
type HandlerFunc func(context.Context, Request) Response

var _ Handler = HandlerFunc(nil)

// Handle process the AdmissionRequest by invoking the underlying function.
func (f HandlerFunc) Handle(ctx context.Context, req Request) Response {
	return f(ctx, req)
}

// Webhook represents each individual webhook.
type Webhook struct {
	// Name is the name of the webhook
	Name string
	// Type is the webhook type, i.e. mutating, validating
	Type WebhookType
	// Path is the path this webhook will serve.
	Path string
	// Handlers contains a list of handlers. Each handler may only contains the business logic for its own feature.
	// For example, feature foo and bar can be in the same webhook if all the other configurations are the same.
	// The handler will be invoked sequentially as the order in the list.
	// Note: if you are using mutating webhook with multiple handlers, it's your responsibility to
	// ensure the handlers are not generating conflicting JSON patches.
	Handlers []Handler

	once sync.Once
}

func (w *Webhook) setDefaults() {
	if len(w.Name) == 0 {
		reg := regexp.MustCompile("[^a-zA-Z0-9]+")
		processedPath := strings.ToLower(reg.ReplaceAllString(w.Path, ""))
		w.Name = processedPath + ".example.com"
	}
}

// Add adds additional handler(s) in the webhook
func (w *Webhook) Add(handlers ...Handler) {
	w.Handlers = append(w.Handlers, handlers...)
}

// Webhook implements Handler interface.
var _ Handler = &Webhook{}

// Handle processes AdmissionRequest.
// If the webhook is mutating type, it delegates the AdmissionRequest to each handler and merge the patches.
// If the webhook is validating type, it delegates the AdmissionRequest to each handler and
// deny the request if anyone denies.
func (w *Webhook) Handle(ctx context.Context, req Request) Response {
	var resp Response
	switch w.Type {
	case MutatingWebhook:
		resp = w.handleMutating(ctx, req)
	case ValidatingWebhook:
		resp = w.handleValidating(ctx, req)
	default:
		return ErrorResponse(http.StatusInternalServerError, errors.New("you must specify your webhook type"))
	}
	resp.UID = req.AdmissionRequest.UID
	return resp
}

func (w *Webhook) handleMutating(ctx context.Context, req Request) Response {
	patches := []jsonpatch.JsonPatchOperation{}
	for _, handler := range w.Handlers {
		resp := handler.Handle(ctx, req)
		if !resp.Allowed {
			setStatusOKInAdmissionResponse(&resp.AdmissionResponse)
			return resp
		}
		if resp.PatchType != nil && *resp.PatchType != admissionv1beta1.PatchTypeJSONPatch {
			return ErrorResponse(http.StatusInternalServerError,
				fmt.Errorf("unexpected patch type returned by the handler: %v, only allow: %v",
					resp.PatchType, admissionv1beta1.PatchTypeJSONPatch))
		}
		patches = append(patches, resp.Patches...)
	}
	var err error
	marshaledPatch, err := json.Marshal(patches)
	if err != nil {
		return ErrorResponse(http.StatusBadRequest, fmt.Errorf("error when marshaling the patch: %v", err))
	}
	return Response{
		AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Code: http.StatusOK,
			},
			Patch:     marshaledPatch,
			PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
		},
	}
}

func (w *Webhook) handleValidating(ctx context.Context, req Request) Response {
	for _, handler := range w.Handlers {
		resp := handler.Handle(ctx, req)
		if !resp.Allowed {
			setStatusOKInAdmissionResponse(&resp.AdmissionResponse)
			return resp
		}
	}
	return Response{
		AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Code: http.StatusOK,
			},
		},
	}
}

func setStatusOKInAdmissionResponse(resp *admissionv1beta1.AdmissionResponse) {
	if resp == nil {
		return
	}
	if resp.Result == nil {
		resp.Result = &metav1.Status{}
	}
	if resp.Result.Code == 0 {
		resp.Result.Code = http.StatusOK
	}
}

// GetName returns the name of the webhook.
func (w *Webhook) GetName() string {
	w.once.Do(w.setDefaults)
	return w.Name
}

// GetPath returns the path that the webhook registered.
func (w *Webhook) GetPath() string {
	w.once.Do(w.setDefaults)
	return w.Path
}

// GetType returns the type of the webhook.
func (w *Webhook) GetType() WebhookType {
	w.once.Do(w.setDefaults)
	return w.Type
}

// Handler returns a http.Handler for the webhook
func (w *Webhook) Handler() http.Handler {
	w.once.Do(w.setDefaults)
	return w
}

// Validate validates if the webhook is valid.
func (w *Webhook) Validate() error {
	w.once.Do(w.setDefaults)
	if len(w.Name) == 0 {
		return errors.New("field Name should not be empty")
	}
	if w.Type != MutatingWebhook && w.Type != ValidatingWebhook {
		return fmt.Errorf("unsupported Type: %v, only MutatingWebhook and ValidatingWebhook are supported", w.Type)
	}
	if len(w.Path) == 0 {
		return errors.New("field Path should not be empty")
	}
	if len(w.Handlers) == 0 {
		return errors.New("field Handler should not be empty")
	}
	return nil
}

// InjectFunc injects the field setter into the webhook.
func (w *Webhook) InjectFunc(f inject.Func) error {
	// inject directly into the handlers.  It would be more correct
	// to do this in a sync.Once in Handle (since we don't have some
	// other start/finalize-type method), but it's more efficient to
	// do it here, presumably.
	for _, handler := range w.Handlers {
		if err := f(handler); err != nil {
			return err
		}
	}
	return nil
}
