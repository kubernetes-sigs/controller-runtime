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

package dashboard

import (
	//applicationv1beta1 "github.com/kubernetes-sigs/application/pkg/apis/app/v1beta1"
	api "sigs.k8s.io/controller-runtime/alpha/patterns/addon/examples/dashboard-operator/pkg/apis/addons/v1alpha1"
	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var _ reconcile.Reconciler = &ReconcileDashboard{}

// ReconcileDashboard reconciles a Dashboard object
type ReconcileDashboard struct {
	declarative.Reconciler
}

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileDashboard {
	// TODO: Dynamic labels?
	labels := map[string]string{
		"k8s-app": "kubernetes-dashboard",
	}

	r := &ReconcileDashboard{}

	/*
		app := applicationv1beta1.Application{
			Spec: applicationv1beta1.ApplicationSpec{
				Descriptor: applicationv1beta1.Descriptor{
					Description: "Kubernetes Dashboard is a general purpose, web-based UI for Kubernetes clusters. It allows users to manage applications running in the cluster and troubleshoot them, as well as manage the cluster itself.",
					Links:       []applicationv1beta1.Link{{Description: "Project Homepage", Url: "https://github.com/kubernetes/dashboard"}},
					Keywords:    []string{"dashboard", "addon"},
				},
			},
		}
	*/

	r.Reconciler.Init(mgr, &api.Dashboard{}, "dashboard",
		declarative.WithObjectTransform(declarative.AddLabels(labels)),
		declarative.WithOwner(declarative.SourceAsOwner),
		//operators.WithGroupVersionKind(api.SchemeGroupVersion.WithKind("dashboard")),
		//operators.WithApplication(app),
	)
	return r
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileDashboard) error {
	// Create a new controller
	c, err := controller.New("dashboard-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Dashboard
	err = c.Watch(&source.Kind{Type: &api.Dashboard{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	/*
		// Watch for changes to deployed objects
		err = r.WatchAllDeployedObjects(c)
		if err != nil {
			return err
		}
	*/

	return nil
}
