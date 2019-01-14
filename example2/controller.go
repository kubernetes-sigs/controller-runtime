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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/example2/logutil"
	"sigs.k8s.io/controller-runtime/example2/pkg"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var fmLog = logutil.Log.WithName("firstmate-reconciler")

// FirstMateController reconciles ReplicaSets
type FirstMateController struct {
	// client can be used to retrieve objects from the APIServer.
	client client.Client
}

func (i *FirstMateController) InjectClient(c client.Client) error {
	i.client = c
	return nil
}

// Implement reconcile.Reconciler so the controller can reconcile objects
var _ reconcile.Reconciler = &FirstMateController{}

func (r *FirstMateController) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// set up a entryLog object so we don't have to type request over and over again
	log := fmLog.WithValues("request", request)
	ctx := context.Background()

	// Fetch the firstMate from the cache
	fm := &pkg.FirstMate{}
	if err := r.client.Get(ctx, request.NamespacedName, fm); errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		log.Error(err, "could not fetch firstMate")
		return reconcile.Result{}, err
	}

	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: request.Name, Namespace: request.Namespace}}
	updateFn := (&createOrUpdateDeployment{firstMate: fm, log: fmLog}).do

	_, err := controllerutil.CreateOrUpdate(ctx, r.client, dep, updateFn)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

type createOrUpdateDeployment struct {
	firstMate *pkg.FirstMate
	log       logr.Logger
}

func (r *createOrUpdateDeployment) do(existing runtime.Object) error {
	r.log.Info("creating or updating deployment")
	dep := existing.(*appsv1.Deployment)
	dep.Labels = r.firstMate.Labels
	dep.Spec.Replicas = &r.firstMate.Spec.Crew
	dep.Spec.Template.Labels = r.firstMate.Labels
	dep.Spec.Selector.MatchLabels = r.firstMate.Labels
	dep.Spec.Template.Spec.Containers = []corev1.Container{{Name: "nginx", Image: "nginx"}}
	return nil
}
