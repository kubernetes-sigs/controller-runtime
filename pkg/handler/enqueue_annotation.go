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

package handler

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ EventHandler = &EnqueueRequestForAnnotation{}

const (
	// NamespacedNameAnnotation defines the annotation that will be used to get the primary resource namespaced name.
	// The handler will use this value to build the types.NamespacedName object used to enqueue a Request when an event
	// to update, to create or to delete the observed object is raised. Note that if only one value be informed without
	// the "/"  then it will be set as the name of the primary resource.
	NamespacedNameAnnotation = "watch.kubebuilder.io/owner-namespaced-name"

	// TypeAnnotation define the annotation that will be used to verify that the primary resource is the primary resource
	// to use. It should be the type schema.GroupKind. E.g watch.kubebuilder.io/owner-type:core.Pods
	TypeAnnotation = "watch.kubebuilder.io/owner-type"
)

// EnqueueRequestForAnnotation enqueues Requests based on the presence of annotations that contain the type and
// namespaced name of the primary resource. The purpose of this handler is to support cross-scope ownership
// relationships that are not supported by native owner references.
//
// This handler should ALWAYS be paired with a finalizer on the primary resource. While the
// annotation-based watch handler does not have the same scope restrictions that owner references
// do, they also do not have the garbage collection guarantees that owner references do. Therefore,
// if the reconciler of a primary resource creates a child resource across scopes not supported by
// owner references, it is up to the reconciler to clean up that child resource.
//
// **NOTE** You might prefer to use the EnqueueRequestsFromMapFunc with predicates instead of it.
//
// **Examples:**
//
// The following code will enqueue a Request to the `primary-resource-namespace` when some change occurs in a Pod
// resource:
//
// ...
// annotation := schema.GroupKind{Group: "ReplicaSet", Kind: "apps"}
// if err := c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForAnnotation{annotation}); err != nil {
//    entryLog.Error(err, "unable to watch Pods")
//    os.Exit(1)
// }
// ...
//
// With the annotations:
//
// ...
// annotations:
//    watch.kubebuilder.io/owner-namespaced-name:my-namespace/my-replicaset
//    watch.kubebuilder.io/owner-type:apps.ReplicaSet
// ...
//
// The following code will enqueue a Request to the `primary-resource-namespace`, ReplicaSet reconcile,
// when some change occurs in a ClusterRole resource
//
// ...
// if err := c.Watch(&source.Kind{
//	  // Watch cluster roles
//	  Type: &rbacv1.ClusterRole{}},
//
//	  // Enqueue ReplicaSet reconcile requests using the
//	  // namespacedName annotation value in the request.
//	  &handler.EnqueueRequestForAnnotation{schema.GroupKind{Group:"ReplicaSet", Kind:"apps"}}); err != nil {
//	      entryLog.Error(err, "unable to watch ClusterRole")
//	      os.Exit(1)
//    }
// }
// ...
// With the annotations:
//
// ...
// annotations:
//    watch.kubebuilder.io/owner-namespaced-name:my-namespace/my-replicaset
//    watch.kubebuilder.io/owner-type:apps.ReplicaSet
// ...
//
// **NOTE** Cluster-scoped resources will have the NamespacedNameAnnotation such as:
// `watch.kubebuilder.io/owner-namespaced-name:my-replicaset
type EnqueueRequestForAnnotation struct {
	// It is used to verify that the primary resource is the primary resource to use.
	// E.g watch.kubebuilder.io/owner-type:core.Pods
	Type schema.GroupKind
}

// Create implements EventHandler
func (e *EnqueueRequestForAnnotation) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	if ok, req := e.getAnnotationRequests(evt.Meta); ok {
		q.Add(req)
	}
}

// Update implements EventHandler
func (e *EnqueueRequestForAnnotation) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if ok, req := e.getAnnotationRequests(evt.MetaOld); ok {
		q.Add(req)
	} else if ok, req := e.getAnnotationRequests(evt.MetaNew); ok {
		q.Add(req)
	}
}

// Delete implements EventHandler
func (e *EnqueueRequestForAnnotation) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	if ok, req := e.getAnnotationRequests(evt.Meta); ok {
		q.Add(req)
	}
}

// Generic implements EventHandler
func (e *EnqueueRequestForAnnotation) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	if ok, req := e.getAnnotationRequests(evt.Meta); ok {
		q.Add(req)
	}
}

// getAnnotationRequests will check if the object has the annotations for the watch handler and requeue
func (e *EnqueueRequestForAnnotation) getAnnotationRequests(object metav1.Object) (bool, reconcile.Request) {
	if typeString, ok := object.GetAnnotations()[TypeAnnotation]; ok && typeString == e.Type.String() {
		namespacedNameString, ok := object.GetAnnotations()[NamespacedNameAnnotation]
		if !ok {
			log.Info("Unable to find the annotation for handle watch annotation",
				"resource", object, "annotation", NamespacedNameAnnotation)
		}
		if len(namespacedNameString) < 1 {
			return false, reconcile.Request{}
		}
		return true, reconcile.Request{NamespacedName: parseNamespacedName(namespacedNameString)}
	}
	return false, reconcile.Request{}
}

// parseNamespacedName will parse the value informed in the NamespacedNameAnnotation and return types.NamespacedName
// with. Note that if just one value is informed, then it will be set as the name.
func parseNamespacedName(namespacedNameString string) types.NamespacedName {
	values := strings.SplitN(namespacedNameString, "/", 2)
	if len(values) == 1 {
		return types.NamespacedName{
			Name:      values[0],
			Namespace: "",
		}
	}
	if len(values) >= 2 {
		return types.NamespacedName{
			Name:      values[1],
			Namespace: values[0],
		}
	}
	return types.NamespacedName{}
}

// SetWatchOwnerAnnotation is a helper method to add the watch annotation-based with the owner NamespacedName and the
// schema.GroupKind to the object provided. This allows you to declare the watch annotations of an owner to an object.
// If a annotation to the same object already exists, it'll be overwritten with the newly provided version.
func SetWatchOwnerAnnotation(owner, object metav1.Object, ownerGK schema.GroupKind) error {

	if len(owner.GetName()) < 1 {
		return fmt.Errorf("%T has not a name, cannot call SetWatchOwnerAnnotation", owner)
	}

	if len(ownerGK.Kind) < 1 {
		return fmt.Errorf("Owner Kind is not found, cannot call SetWatchOwnerAnnotation")
	}

	if len(ownerGK.Group) < 1 {
		return fmt.Errorf("Owner Group is not found, cannot call SetWatchOwnerAnnotation")
	}

	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[NamespacedNameAnnotation] = fmt.Sprintf("%v/%v", owner.GetNamespace(), owner.GetName())
	annotations[TypeAnnotation] = ownerGK.String()
	object.SetAnnotations(annotations)

	return nil
}
