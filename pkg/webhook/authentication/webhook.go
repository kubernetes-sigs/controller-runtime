/*
Copyright 2021 The Kubernetes Authors.

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

package authentication

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
	authenticationv1 "k8s.io/api/authentication/v1"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

var (
	errUnableToEncodeResponse = errors.New("unable to encode response")
)

// Request defines the input for an authentication handler.
// It contains information to identify the object in
// question (group, version, kind, resource, subresource,
// name, namespace), as well as the operation in question
// (e.g. Get, Create, etc), and the object itself.
type Request struct {
	authenticationv1.TokenReview
}

// Response is the output of an authentication handler.
// It contains a response indicating if a given
// operation is allowed.
type Response struct {
	authenticationv1.TokenReview
}

// Complete populates any fields that are yet to be set in
// the underlying TokenResponse, It mutates the response.
func (r *Response) Complete(req Request) error {
	r.UID = req.UID

	return nil
}

// Handler can handle an TokenReview.
type Handler interface {
	// Handle yields a response to an TokenReview.
	//
	// The supplied context is extracted from the received http.Request, allowing wrapping
	// http.Handlers to inject values into and control cancelation of downstream request processing.
	Handle(context.Context, Request) Response
}

// HandlerFunc implements Handler interface using a single function.
type HandlerFunc func(context.Context, Request) Response

var _ Handler = HandlerFunc(nil)

// Handle process the TokenReview by invoking the underlying function.
func (f HandlerFunc) Handle(ctx context.Context, req Request) Response {
	return f(ctx, req)
}

// Webhook represents each individual webhook.
type Webhook struct {
	// Handler actually processes an authentication request returning whether it was authenticated or unauthenticated,
	// and potentially patches to apply to the handler.
	Handler Handler

	// WithContextFunc will allow you to take the http.Request.Context() and
	// add any additional information such as passing the request path or
	// headers thus allowing you to read them from within the handler
	WithContextFunc func(context.Context, *http.Request) context.Context

	log logr.Logger
}

// InjectLogger gets a handle to a logging instance, hopefully with more info about this particular webhook.
func (wh *Webhook) InjectLogger(l logr.Logger) error {
	wh.log = l
	return nil
}

// Handle processes TokenReview.
func (wh *Webhook) Handle(ctx context.Context, req Request) Response {
	resp := wh.Handler.Handle(ctx, req)
	if err := resp.Complete(req); err != nil {
		wh.log.Error(err, "unable to encode response")
		return Errored(errUnableToEncodeResponse)
	}

	return resp
}

// InjectFunc injects the field setter into the webhook.
func (wh *Webhook) InjectFunc(f inject.Func) error {
	// inject directly into the handlers.  It would be more correct
	// to do this in a sync.Once in Handle (since we don't have some
	// other start/finalize-type method), but it's more efficient to
	// do it here, presumably.

	var setFields inject.Func
	setFields = func(target interface{}) error {
		if err := f(target); err != nil {
			return err
		}

		if _, err := inject.InjectorInto(setFields, target); err != nil {
			return err
		}

		return nil
	}

	return setFields(wh.Handler)
}
