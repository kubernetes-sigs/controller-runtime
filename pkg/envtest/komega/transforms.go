/*
Copyright 2021 The Kubernetes Authors.

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

package komega

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/onsi/gomega"
	gtypes "github.com/onsi/gomega/types"
)

// WithField gets the value of the named field from the object.
// This is intended to be used in assertions with the Matcher make it easy
// to check the value of a particular field in a resource.
// To access nested fields uses a `.` separator.
// Eg.
//    m.Eventually(deployment).Should(WithField("spec.replicas", BeZero()))
// To access nested lists, use one of the Gomega list matchers in conjunction with this.
// Eg.
//    m.Eventually(deploymentList).Should(WithField("items", ConsistOf(...)))
func WithField(field string, matcher gtypes.GomegaMatcher) gtypes.GomegaMatcher {
	// Addressing Field by <struct>.<field> can be recursed
	fields := strings.SplitN(field, ".", 2)
	if len(fields) == 2 {
		matcher = WithField(fields[1], matcher)
	}

	return gomega.WithTransform(func(obj interface{}) interface{} {
		r := reflect.ValueOf(obj)
		f := reflect.Indirect(r).FieldByName(fields[0])
		if !f.IsValid() {
			panic(fmt.Sprintf("Object '%s' does not have a field '%s'", reflect.TypeOf(obj), fields[0]))
		}
		return f.Interface()
	}, matcher)
}
