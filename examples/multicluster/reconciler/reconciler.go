/*
Copyright 2020 The Kubernetes Authors.

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

package reconciler

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/clusterconnector"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Add(mgr ctrl.Manager, mirrorCluster clusterconnector.ClusterConnector) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Watch Pods in the reference cluster
		For(&corev1.Pod{}).
		// Watch pods in the mirror cluster
		Watches(
			source.NewKindWithCache(&corev1.Pod{}, mirrorCluster.GetCache()),
			&handler.EnqueueRequestForObject{},
		).
		Complete(&reconciler{
			referenceClusterClient: mgr.GetClient(),
			mirrorClusterClient:    mirrorCluster.GetClient(),
		})
}

type reconciler struct {
	referenceClusterClient client.Client
	mirrorClusterClient    client.Client
}

func (r *reconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, r.reconcile(req)
}

const podFinalizerName = "pod-finalzer.mirror.org/v1"

func (r *reconciler) reconcile(req reconcile.Request) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	referencePod := &corev1.Pod{}
	if err := r.referenceClusterClient.Get(ctx, req.NamespacedName, referencePod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get pod from reference clsuster: %w", err)
	}

	if referencePod.DeletionTimestamp != nil && sets.NewString(referencePod.Finalizers...).Has(podFinalizerName) {
		mirrorClusterPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: referencePod.Namespace,
			Name:      referencePod.Name,
		}}
		if err := r.mirrorClusterClient.Delete(ctx, mirrorClusterPod); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete pod in mirror cluster: %w", err)
		}

		referencePod.Finalizers = sets.NewString(referencePod.Finalizers...).Delete(podFinalizerName).UnsortedList()
		if err := r.referenceClusterClient.Update(ctx, referencePod); err != nil {
			return fmt.Errorf("failed to update pod in refernce cluster after removing finalizer: %w", err)
		}

		return nil
	}

	if !sets.NewString(referencePod.Finalizers...).Has(podFinalizerName) {
		referencePod.Finalizers = append(referencePod.Finalizers, podFinalizerName)
		if err := r.referenceClusterClient.Update(ctx, referencePod); err != nil {
			return fmt.Errorf("failed to update pod after adding finalizer: %w", err)
		}
	}

	// Check if pod already exists
	podName := types.NamespacedName{Namespace: referencePod.Namespace, Name: referencePod.Name}
	podExists := true
	if err := r.mirrorClusterClient.Get(ctx, podName, &corev1.Pod{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to check in mirror cluster if pod exists: %w", err)
		}
		podExists = false
	}
	if podExists {
		return nil
	}

	mirrorPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: referencePod.Namespace,
			Name:      referencePod.Name,
		},
		Spec: *referencePod.Spec.DeepCopy(),
	}
	if err := r.mirrorClusterClient.Create(ctx, mirrorPod); err != nil {
		return fmt.Errorf("failed to create mirror pod: %w", err)
	}

	return nil
}
