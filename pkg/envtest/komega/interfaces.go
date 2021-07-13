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
	"context"
	"time"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Komega is the root interface that the Matcher implements.
type Komega interface {
	KomegaAsync
	KomegaSync
	WithContext(context.Context) Komega
}

// KomegaSync is the interface for any sync assertions that
// the matcher implements.
type KomegaSync interface {
	Create(client.Object, ...client.CreateOption) gomega.GomegaAssertion
	Delete(client.Object, ...client.DeleteOption) gomega.GomegaAssertion
	WithExtras(...interface{}) KomegaSync
}

// KomegaAsync is the interface for any async assertions that
// the matcher implements.
type KomegaAsync interface {
	Consistently(runtime.Object, ...client.ListOption) gomega.AsyncAssertion
	Eventually(runtime.Object, ...client.ListOption) gomega.AsyncAssertion
	Get(client.Object) gomega.AsyncAssertion
	List(client.ObjectList, ...client.ListOption) gomega.AsyncAssertion
	Update(client.Object, UpdateFunc, ...client.UpdateOption) gomega.AsyncAssertion
	UpdateStatus(client.Object, UpdateFunc, ...client.UpdateOption) gomega.AsyncAssertion
	WithTimeout(time.Duration) KomegaAsync
	WithPollInterval(time.Duration) KomegaAsync
}

// UpdateFunc modifies the object fetched from the API server before sending
// the update
type UpdateFunc func(client.Object) client.Object
