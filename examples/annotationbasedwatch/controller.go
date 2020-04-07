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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// reconcileReplicaSet reconciles ReplicaSets
type reconcileReplicaSet struct {
	// client can be used to retrieve objects from the APIServer.
	client client.Client
	log    logr.Logger
}

// Implement reconcile.Reconciler so the controller can reconcile objects
var _ reconcile.Reconciler = &reconcileReplicaSet{}

func (r *reconcileReplicaSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// set up a convenient log object so we don't have to type request over and over again
	log := r.log.WithValues("request", request)

	// Fetch the ReplicaSet from the cache
	rs := &appsv1.ReplicaSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, rs)
	if errors.IsNotFound(err) {
		log.Error(nil, "Could not find ReplicaSet")
		return reconcile.Result{}, nil
	}

	if err != nil {
		log.Error(err, "Could not fetch ReplicaSet")
		return reconcile.Result{}, err
	}

	// Print the ReplicaSet
	log.Info("Reconciling ReplicaSet", "container name", rs.Spec.Template.Spec.Containers[0].Name)

	// Check if the Pod already exists, if not create a new one
	podRs := &corev1.Pod{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: rs.Name, Namespace: rs.Namespace}, podRs)
	if err != nil && errors.IsNotFound(err) {
		// Define a new Deployment
		pod := r.podForReplicasetWithWatchAnnotations(rs)
		err = r.client.Create(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Failed to create new Pod.", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Pod.")
		return reconcile.Result{}, err
	}

	// Set the label if it is missing
	if rs.Labels == nil {
		rs.Labels = map[string]string{}
	}

	if rs.Labels["hello"] == "world" {
		return reconcile.Result{}, nil
	}

	// Update the ReplicaSet
	rs.Labels["hello"] = "world"
	err = r.client.Update(context.TODO(), rs)
	if err != nil {
		log.Error(err, "Could not write ReplicaSet")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// podForReplicasetWithWatchAnnotations returns a pod object with the annotations required to be watched with.
func (r *reconcileReplicaSet) podForReplicasetWithWatchAnnotations(rs *appsv1.ReplicaSet) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name,
			Namespace: rs.Namespace,
		},
	}
	annotation := schema.GroupKind{Group: "ReplicaSet", Kind: "apps"}
	handler.SetWatchOwnerAnnotation(rs,pod, annotation)
	return pod
}