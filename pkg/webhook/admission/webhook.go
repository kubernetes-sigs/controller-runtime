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
	"sync"

	"github.com/mattbaird/jsonpatch"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

// StringKey is the type used as key in the KVMap
type StringKey string

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
// Webhook implements Handler interface.
type Webhook struct {
	// Name is the name of the webhook
	Name string
	// Type is the webhook type, i.e. mutating, validating
	Type types.WebhookType
	// Path is the path this webhook will serve.
	Path string
	// Rules maps to the Rules field in admissionregistrationv1beta1.Webhook
	Rules []admissionregistrationv1beta1.RuleWithOperations
	// FailurePolicy maps to the FailurePolicy field in admissionregistrationv1beta1.Webhook
	// This optional. If not set, will be defaulted to Ignore (fail-open).
	// More details: https://github.com/kubernetes/api/blob/f5c295feaba2cbc946f0bbb8b535fc5f6a0345ee/admissionregistration/v1beta1/types.go#L144-L147
	FailurePolicy *admissionregistrationv1beta1.FailurePolicyType
	// NamespaceSelector maps to the NamespaceSelector field in admissionregistrationv1beta1.Webhook
	// This optional.
	NamespaceSelector *metav1.LabelSelector
	// KVMap contains key-value pairs that will be passed to the handler for processing the Request.
	KVMap map[string]interface{}
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
		w.Name = "admissionwebhook.k8s.io"
	}
	if len(w.Path) == 0 {
		if w.Type == types.WebhookTypeMutating {
			w.Path = "/mutatingwebhook"
		} else if w.Type == types.WebhookTypeValidating {
			w.Path = "/validatingwebhook"
		}
	}
}

// Add adds additional handler(s) in the webhook
func (w *Webhook) Add(handlers ...Handler) {
	w.Handlers = append(w.Handlers, handlers...)
}

var _ Handler = &Webhook{}

// Handle processes AdmissionRequest.
// If the webhook is mutating type, it delegates the AdmissionRequest to each handler and merge the patches.
// If the webhook is validating type, it delegates the AdmissionRequest to each handler and
// deny the request if anyone denies.
func (w *Webhook) Handle(ctx context.Context, req Request) Response {
	if req.AdmissionRequest == nil {
		return ErrorResponse(http.StatusBadRequest, errors.New("got an empty AdmissionRequest"))
	}
	var resp Response
	var err error
	ctx, err = w.kvMapToContext(ctx)
	if err != nil {
		return ErrorResponse(http.StatusInternalServerError, fmt.Errorf("error when preparing the context: %v", err))
	}
	switch w.Type {
	case types.WebhookTypeMutating:
		resp = w.handleMutating(ctx, req)
	case types.WebhookTypeValidating:
		resp = w.handleValidating(ctx, req)
	default:
		return ErrorResponse(http.StatusInternalServerError, errors.New("you must specify your webhook type"))
	}
	resp.Response.UID = req.AdmissionRequest.UID
	return resp
}

// kvMapToContext passes the key value pair from the KVMap to the context for processing the request.
func (w *Webhook) kvMapToContext(ctx context.Context) (context.Context, error) {
	for k, v := range w.KVMap {
		if existing := ctx.Value(k); existing != nil && existing != v {
			return nil, fmt.Errorf("key %v already exists in the context, the value is %v", k, existing)
		}
		ctx = context.WithValue(ctx, StringKey(k), v)
	}
	return ctx, nil
}

func (w *Webhook) handleMutating(ctx context.Context, req Request) Response {
	patches := []jsonpatch.JsonPatchOperation{}
	for _, handler := range w.Handlers {
		resp := handler.Handle(ctx, req)
		if !resp.Response.Allowed {
			return resp
		}
		if resp.Response.PatchType != nil && *resp.Response.PatchType != admissionv1beta1.PatchTypeJSONPatch {
			return ErrorResponse(http.StatusInternalServerError,
				fmt.Errorf("unexpected patch type returned by the handler: %v, only allow: %v",
					resp.Response.PatchType, admissionv1beta1.PatchTypeJSONPatch))
		}
		patches = append(patches, resp.Patches...)
	}
	var err error
	marshaledPatch, err := json.Marshal(patches)
	if err != nil {
		return ErrorResponse(http.StatusBadRequest, fmt.Errorf("error when marshaling the patch: %v", err))
	}
	return Response{
		Response: &admissionv1beta1.AdmissionResponse{
			Allowed:   true,
			Patch:     marshaledPatch,
			PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
		},
	}
}

func (w *Webhook) handleValidating(ctx context.Context, req Request) Response {
	for _, handler := range w.Handlers {
		resp := handler.Handle(ctx, req)
		if !resp.Response.Allowed {
			return resp
		}
	}
	return Response{
		Response: &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		},
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
func (w *Webhook) GetType() types.WebhookType {
	w.once.Do(w.setDefaults)
	return w.Type
}

// Handler returns a http.Handler for the webhook
func (w *Webhook) Handler() http.Handler {
	w.once.Do(w.setDefaults)
	return w
}

// SetKVMap sets the KVMap of the webhook.
func (w *Webhook) SetKVMap(m map[string]interface{}) {
	w.KVMap = m
}

// Validate validates if the webhook is valid.
func (w *Webhook) Validate() error {
	w.once.Do(w.setDefaults)
	if len(w.Name) == 0 {
		return errors.New("field Name should not be empty")
	}
	if w.Type != types.WebhookTypeMutating && w.Type != types.WebhookTypeValidating {
		return fmt.Errorf("unsupported Type: %v, only WebhookTypeMutating and WebhookTypeValidating are supported", w.Type)
	}
	if len(w.Path) == 0 {
		return errors.New("field Path should not be empty")
	}
	if len(w.Rules) == 0 {
		return errors.New("field Rules should not be empty")
	}
	if len(w.Handlers) == 0 {
		return errors.New("field Handler should not be empty")
	}
	return nil
}
