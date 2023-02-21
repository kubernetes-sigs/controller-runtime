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

package controllerruntime_test

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// since we invoke tests with -ginkgo.junit-report we need to import ginkgo.
	_ "github.com/onsi/ginkgo/v2"
)

// This example creates a simple application Controller that is configured for ReplicaSets and Pods.
//
// * Create a new application for ReplicaSets that manages Pods owned by the ReplicaSet and calls into
// ReplicaSetReconciler.
//
// * Start the application.
func Example() {
	var log = ctrl.Log.WithName("builder-examples")

	manager, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		log.Error(err, "could not create manager")
		os.Exit(1)
	}

	err = ctrl.
		NewControllerManagedBy(manager). // Create the Controller
		For(&appsv1.ReplicaSet{}).       // ReplicaSet is the Application API
		Owns(&corev1.Pod{}).             // ReplicaSet owns Pods created by it
		Complete(&ReplicaSetReconciler{Client: manager.GetClient()})
	if err != nil {
		log.Error(err, "could not create controller")
		os.Exit(1)
	}

	if err := manager.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "could not start manager")
		os.Exit(1)
	}
}

// This example to test event broadcaster to set qps and burst size.
func ExampleEventBroadcaster() {
	var examplePod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
	}
	var log = ctrl.Log.WithName("event-examples")
	// Controller have two diff recorderProvider
	// 1.manager will create recorderProvider to resourceLock and push lease event.
	// 2.cluster also create recorderProvider,when call manager.GetEventRecorderFor,will get the provider.
	// When use the provider to push event like eventf(),provider will call getBroadcaster and StartEventWatcher.
	// If use the EventBroadcaster options,the diff recorderProvider's makeBroadcaster func will return same broadcaster point
	// Then call broadcaster.StartEventWatcher func duplicate,event will dunplicate push.
	manager, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		LeaderElectionID:        "123456qbcdqe",
		LeaderElection:          true,
		LeaderElectionNamespace: "test1",
		EventBroadcaster: record.NewBroadcasterWithCorrelatorOptions(record.CorrelatorOptions{
			BurstSize: 50,
			QPS:       15,
		}),
	})
	if err != nil {
		log.Error(err, "could not create manager")
		os.Exit(1)
	}
	go func() {
		for {
			select {
			case <-ctrl.SetupSignalHandler().Done():
				return
			case <-time.After(time.Second * 10):
				er := manager.GetEventRecorderFor("event-test")
				// Like this,will push the same event two times.
				er.Eventf(examplePod, corev1.EventTypeNormal, "event-test", "EventBroadcaster test")
				log.V(1).Info("eventf end")
			}
		}
	}()
	if err := manager.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "could not start manager")
		os.Exit(1)
	}
	log.Info("success")
}

// This example creates a simple application Controller that is configured for ReplicaSets and Pods.
// This application controller will be running leader election with the provided configuration in the manager options.
// If leader election configuration is not provided, controller runs leader election with default values.
// Default values taken from: https://github.com/kubernetes/component-base/blob/master/config/v1alpha1/defaults.go
// * defaultLeaseDuration = 15 * time.Second
// * defaultRenewDeadline = 10 * time.Second
// * defaultRetryPeriod   = 2 * time.Second
//
// * Create a new application for ReplicaSets that manages Pods owned by the ReplicaSet and calls into
// ReplicaSetReconciler.
//
// * Start the application.
func Example_updateLeaderElectionDurations() {
	var log = ctrl.Log.WithName("builder-examples")
	leaseDuration := 100 * time.Second
	renewDeadline := 80 * time.Second
	retryPeriod := 20 * time.Second
	manager, err := ctrl.NewManager(
		ctrl.GetConfigOrDie(),
		ctrl.Options{
			LeaseDuration: &leaseDuration,
			RenewDeadline: &renewDeadline,
			RetryPeriod:   &retryPeriod,
		})
	if err != nil {
		log.Error(err, "could not create manager")
		os.Exit(1)
	}

	err = ctrl.
		NewControllerManagedBy(manager). // Create the Controller
		For(&appsv1.ReplicaSet{}).       // ReplicaSet is the Application API
		Owns(&corev1.Pod{}).             // ReplicaSet owns Pods created by it
		Complete(&ReplicaSetReconciler{Client: manager.GetClient()})
	if err != nil {
		log.Error(err, "could not create controller")
		os.Exit(1)
	}

	if err := manager.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "could not start manager")
		os.Exit(1)
	}
}

// ReplicaSetReconciler is a simple Controller example implementation.
type ReplicaSetReconciler struct {
	client.Client
}

// Implement the business logic:
// This function will be called when there is a change to a ReplicaSet or a Pod with an OwnerReference
// to a ReplicaSet.
//
// * Read the ReplicaSet
// * Read the Pods
// * Set a Label on the ReplicaSet with the Pod count.
func (a *ReplicaSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Read the ReplicaSet
	rs := &appsv1.ReplicaSet{}
	err := a.Get(ctx, req.NamespacedName, rs)
	if err != nil {
		return ctrl.Result{}, err
	}

	// List the Pods matching the PodTemplate Labels
	pods := &corev1.PodList{}
	err = a.List(ctx, pods, client.InNamespace(req.Namespace), client.MatchingLabels(rs.Spec.Template.Labels))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update the ReplicaSet
	rs.Labels["pod-count"] = fmt.Sprintf("%v", len(pods.Items))
	err = a.Update(ctx, rs)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
