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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("example-annotationbasedwatch")

func main() {
	logf.SetLogger(zap.Logger(false))
	entryLog := log.WithName("entrypoint")

	// Setup a Manager
	entryLog.Info("setting up manager")
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	// Setup a new controller to reconcile ReplicaSets
	entryLog.Info("Setting up controller")
	c, err := controller.New("foo-controller", mgr, controller.Options{
		Reconciler: &reconcileReplicaSet{client: mgr.GetClient(), log: log.WithName("reconciler")},
	})
	if err != nil {
		entryLog.Error(err, "unable to set up individual controller")
		os.Exit(1)
	}

	// Watch Pods that has the following annotations:
	// ...
	// annotations:
	//    watch.kubebuilder.io/owner-namespaced-name:my-namespace/my-replicaset
	//    watch.kubebuilder.io/owner-type:apps.ReplicaSet
	// ...
	// It will enqueue a Request to the primary-resource-namespace when some change occurs in a Pod resource with these
	// annotations
	annotation := schema.GroupKind{Group: "ReplicaSet", Kind: "apps"}
	if err := c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForAnnotation{annotation}); err != nil {
		entryLog.Error(err, "unable to watch Pods")
		os.Exit(1)
	}

	// Watch ClusterRoles that has the following annotations:
	// ...
	// annotations:
	//    watch.kubebuilder.io/owner-namespaced-name:my-namespace/my-replicaset
	//    watch.kubebuilder.io/owner-type:apps.ReplicaSet
	// ...
	// It will enqueue a Request to the primary-resource-namespace when some change occurs in a ClusterRole resource with these
	// annotations
	if err := c.Watch(&source.Kind{
		// Watch cluster roles
		Type: &rbacv1.ClusterRole{}},

		// Enqueue ReplicaSet reconcile requests using the
		// namespacedName annotation value in the request.
		&handler.EnqueueRequestForAnnotation{schema.GroupKind{Group:"ReplicaSet", Kind:"apps"}}); err != nil {
		entryLog.Error(err, "unable to watch Cluster Role")
		os.Exit(1)
	}

	entryLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}
