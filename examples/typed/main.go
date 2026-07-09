/*
Copyright The Kubernetes Authors.

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
	"os"

	networkingv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		return fmt.Errorf("failed to construct manager: %w", err)
	}

	// Use a request type that is always equal to itself so the workqueue
	// de-duplicates all events.
	// This can for example be useful for an ingress-controller that
	// generates a config from all ingresses, rather than individual ones.
	type request struct{}

	r := reconcile.TypedFunc[request](func(ctx context.Context, _ request) (reconcile.Result, error) {
		ingressList := &networkingv1.IngressList{}
		if err := mgr.GetClient().List(ctx, ingressList); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to list ingresses: %w", err)
		}

		buildIngressConfig(ingressList)
		return reconcile.Result{}, nil
	})
	if err := builder.TypedControllerManagedBy[request](mgr).
		WatchesRawSource(source.TypedKind(
			mgr.GetCache(),
			&networkingv1.Ingress{},
			handler.TypedEnqueueRequestsFromMapFunc(func(context.Context, *networkingv1.Ingress) []request {
				return []request{{}}
			})),
		).
		Named("ingress_controller").
		Complete(r); err != nil {
		return fmt.Errorf("failed to construct ingress-controller: %w", err)
	}

	ctx := signals.SetupSignalHandler()
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	return nil
}

func buildIngressConfig(*networkingv1.IngressList) {}
