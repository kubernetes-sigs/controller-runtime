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

package admission

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
)

// DecodeLegacy decodes the inlined object in the AdmissionRequest into the passed-in runtime.Object.
// If you want decode the OldObject in the AdmissionRequest, use DecodeRaw.
// It errors out if req.Object.Raw is empty i.e. containing 0 raw bytes.
func (d *Decoder) DecodeLegacy(req RequestLegacy, into runtime.Object) error {
	// we error out if rawObj is an empty object.
	if len(req.Object.Raw) == 0 {
		return fmt.Errorf("there is no content to decode")
	}
	return d.DecodeRaw(req.Object, into)
}
