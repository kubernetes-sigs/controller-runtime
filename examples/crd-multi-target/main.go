/*
Copyright 2023 The Kubernetes Authors.

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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	api "sigs.k8s.io/controller-runtime/examples/crd-multi-target/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

type reconciler struct {
	// client can be used to retrieve objects from the APIServer.
	client client.Client

	scheme *runtime.Scheme
}

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("MyReplicaSet", req.NamespacedName)
	log.Info("reconciling", "source", req.Source)

	// Fetch the ReplicaSet from the cache
	rs := &api.MyReplicaSet{}
	if err := r.client.Get(ctx, req.NamespacedName, rs); apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("could not get MyReplicaSet: %w", err)
	}

	expectedPodNames := map[string]bool{}
	for i := 0; i < int(*rs.Spec.Replicas); i++ {
		expectedPodNames[fmt.Sprintf("%s-%d", rs.Name, i)] = true
	}

	if req.Source == nil { // If the source is not specified, we reconcile all pods linked to this ReplicaSet
		// List the pods for this rs's deployment
		podList := &api.MyPodList{}

		selector, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("could not convert selector: %w", err)
		}

		listOpts := []client.ListOption{
			client.InNamespace(req.Namespace),
			client.MatchingLabelsSelector{
				Selector: selector,
			},
		}

		if err := r.client.List(ctx, podList, listOpts...); err != nil {
			return reconcile.Result{}, fmt.Errorf("could not list pods: %w", err)
		}

		log.Info("listed pods", "count", len(podList.Items))
		for _, pod := range podList.Items {
			// Delete the pod if it is not in the expected list
			if !expectedPodNames[pod.Name] {
				log.Info("removing unwanted pod", "name", pod.Name)
				if err := r.client.Delete(ctx, &pod); err != nil {
					return reconcile.Result{}, fmt.Errorf("could not delete pod: %w", err)
				}
			} else {
				log.Info("updating expected pod", "name", pod.Name)

				updatedPod := updatedPodClone(&pod, pod.Name, rs)

				if !reflect.DeepEqual(&pod, updatedPod) {
					log.Info("updating existing pod", "name", pod.Name)

					// Update the pod
					if err := r.client.Update(ctx, updatedPod); err != nil {
						return reconcile.Result{}, fmt.Errorf("could not update pod: %w", err)
					}
				} else {
					log.Info("individual pod is up to date", "name", pod.Name)
				}

				// Remove the pod from the expected list
				delete(expectedPodNames, pod.Name)
			}
		}

		// Create the pod if it is not in the list
		for podName := range expectedPodNames {
			log.Info("creating missing pod", "name", podName)

			pod := updatedPodClone(&api.MyPod{}, podName, rs)

			if err := r.client.Create(ctx, pod); apierrors.IsAlreadyExists(err) {
				log.Info("failed to create missing pod because pod already exists", "name", podName)

				// If the pod already exists, we update it; this is caused by incorrect
				// labels/ label selector in the ReplicaSet

				// Get the existing pod
				existingPod := &api.MyPod{}
				if err := r.client.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name}, existingPod); err != nil {
					return reconcile.Result{}, fmt.Errorf("could not get pod: %w", err)
				}

				updatedPod := updatedPodClone(existingPod, podName, rs)

				if !reflect.DeepEqual(existingPod, updatedPod) {
					log.Info("updating existing pod", "name", podName)

					// Update the pod
					if err := r.client.Update(ctx, updatedPod); err != nil {
						return reconcile.Result{}, fmt.Errorf("could not update pod: %w", err)
					}
				} else {
					log.Info("individual pod is up to date", "name", podName)
				}
			} else if err != nil {
				return reconcile.Result{}, fmt.Errorf("could not create pod: %w", err)
			}
		}

		return reconcile.Result{}, nil
	} else { // If the source is specified, we reconcile only the specified pod
		if req.Source.Kind != "MyPod" || req.Source.GroupVersion() != api.SchemeGroupVersion {
			log.Error(nil, "unexpected source", "source", req.Source)
			return reconcile.Result{}, nil
		}

		pod := &api.MyPod{}
		if err := r.client.Get(ctx, req.Source.NamespacedName, pod); apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		} else if err != nil {
			return reconcile.Result{}, fmt.Errorf("could not get pod: %w", err)
		}

		// If the pod is not in the expected list, we delete it
		if !expectedPodNames[pod.GetName()] {
			log.Info("remove individual unnecessary pod", "name", pod.GetName())

			if err := r.client.Delete(ctx, pod); err != nil {
				return reconcile.Result{}, fmt.Errorf("could not delete pod: %w", err)
			}

			return reconcile.Result{}, nil
		} else {
			updatedPod := updatedPodClone(pod, pod.Name, rs)

			if !reflect.DeepEqual(pod, updatedPod) {
				log.Info("updating individual existing pod", "name", pod.GetName())

				// Update the pod
				if err := r.client.Update(ctx, updatedPod); err != nil {
					return reconcile.Result{}, fmt.Errorf("could not update pod: %w", err)
				}
			} else {
				log.Info("individual pod is up to date", "name", pod.GetName())
			}

			return reconcile.Result{}, nil
		}
	}
}

func updatedPodClone(mypod *api.MyPod, podName string, rs *api.MyReplicaSet) *api.MyPod {
	mypod = mypod.DeepCopy()
	mypod.SetName(podName)
	mypod.SetNamespace(rs.Namespace)
	mypod.SetLabels(rs.Spec.Template.ObjectMeta.GetLabels())
	mypod.SetAnnotations(rs.Spec.Template.ObjectMeta.GetAnnotations())
	mypod.SetOwnerReferences(
		[]metav1.OwnerReference{
			{
				APIVersion:         api.SchemeGroupVersion.String(),
				Kind:               "MyReplicaSet",
				Name:               rs.Name,
				UID:                rs.UID,
				Controller:         &[]bool{true}[0],
				BlockOwnerDeletion: &[]bool{true}[0],
			},
		},
	)
	mypod.Spec = rs.Spec.Template.Spec
	return mypod
}

func main() {
	ctrl.SetLogger(zap.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// in a real controller, we'd create a new scheme for this
	err = api.AddToScheme(mgr.GetScheme())
	if err != nil {
		setupLog.Error(err, "unable to add scheme")
		os.Exit(1)
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&api.MyReplicaSet{}).
		Owns(&api.MyPod{}).
		Complete(&reconciler{
			client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
		})
	if err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
