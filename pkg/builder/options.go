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

package builder

import "k8s.io/apimachinery/pkg/runtime"

// OnlyMetadata tells the controller to *only* cache metadata, and to watch
// the the API server in metadata-only form.  This is useful when watching
// lots of objects, really big objects, or objects for which you only know
// the the GVK, but not the structure.  You'll need to pass
// metav1.PartialObjectMetadata to the client when fetching objects in your
// reconciler, otherwise you'll end up with a duplicate structured or
// unstructured cache.
func OnlyMetadata(obj runtime.Object) runtime.Object {
	return &onlyMetadataWrapper{obj}
}

type onlyMetadataWrapper struct {
	runtime.Object
}

// }}}
