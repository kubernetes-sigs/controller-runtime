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
	"fmt"
	"net/http"
	"sync"

	"github.com/go-logr/logr"
	authenticationv1 "k8s.io/api/authentication/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	authenticationmetrics "sigs.k8s.io/controller-runtime/pkg/webhook/authentication/metrics"
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

	// RecoverPanic indicates whether the panic caused by webhook should be recovered.
	// Defaults to true.
	RecoverPanic *bool

	// WithContextFunc will allow you to take the http.Request.Context() and
	// add any additional information such as passing the request path or
	// headers thus allowing you to read them from within the handler
	WithContextFunc func(context.Context, *http.Request) context.Context

	setupLogOnce sync.Once
	log          logr.Logger
}

// WithRecoverPanic takes a bool flag which indicates whether the panic caused by webhook should be recovered.
// Defaults to true.
func (wh *Webhook) WithRecoverPanic(recoverPanic bool) *Webhook {
	wh.RecoverPanic = &recoverPanic
	return wh
}

// Handle processes TokenReview.
func (wh *Webhook) Handle(ctx context.Context, req Request) (response Response) {
	defer func() {
		if r := recover(); r != nil {
			authenticationmetrics.WebhookPanics.Inc()

			if wh.RecoverPanic == nil || *wh.RecoverPanic {
				for _, fn := range utilruntime.PanicHandlers {
					fn(ctx, r)
				}
				response = Errored(fmt.Errorf("panic: %v [recovered]", r))
				// Note: We explicitly have to set the response UID. Usually that is done via resp.Complete below,
				// but if we encounter a panic in wh.Handler.Handle we are never going to reach resp.Complete.
				response.UID = req.UID
				return
			}

			log := logf.FromContext(ctx)
			log.Info(fmt.Sprintf("Observed a panic in webhook: %v", r))
			panic(r)
		}
	}()

	resp := wh.Handler.Handle(ctx, req)
	if err := resp.Complete(req); err != nil {
		wh.getLogger(&req).Error(err, "unable to encode response")
		return Errored(errUnableToEncodeResponse)
	}

	return resp
}

// getLogger constructs a logger from the injected log and LogConstructor.
func (wh *Webhook) getLogger(req *Request) logr.Logger {
	wh.setupLogOnce.Do(func() {
		if wh.log.GetSink() == nil {
			wh.log = logf.Log.WithName("authentication")
		}
	})

	return logConstructor(wh.log, req)
}

// logConstructor adds some commonly interesting fields to the given logger.
func logConstructor(base logr.Logger, req *Request) logr.Logger {
	if req != nil {
		return base.WithValues("object", klog.KRef(req.Namespace, req.Name),
			"namespace", req.Namespace, "name", req.Name,
			"user", req.Status.User.Username,
			"requestID", req.UID,
		)
	}
	return base
}
