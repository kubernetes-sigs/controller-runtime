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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Matcher has Gomega Matchers that use the controller-runtime client.
type Matcher struct {
	Client       client.Client
	ctx          context.Context
	extras       []interface{}
	timeout      time.Duration
	pollInterval time.Duration
}

// WithContext sets the context to be used for the underlying client
// during assertions.
func (m *Matcher) WithContext(ctx context.Context) Komega {
	m.ctx = ctx
	return m
}

// context returns the matcher context if one has been set.
// Else it returns the context.TODO().
func (m *Matcher) context() context.Context {
	if m.ctx == nil {
		return context.TODO()
	}
	return m.ctx
}

// WithExtras sets extra arguments for sync assertions.
// Any extras passed will be expected to be nil during assertion.
func (m *Matcher) WithExtras(extras ...interface{}) KomegaSync {
	m.extras = extras
	return m
}

// WithTimeout sets the timeout for any async assertions.
func (m *Matcher) WithTimeout(timeout time.Duration) KomegaAsync {
	m.timeout = timeout
	return m
}

// WithPollInterval sets the poll interval for any async assertions.
// Note: This will only work if an explicit timeout has been set with WithTimeout.
func (m *Matcher) WithPollInterval(pollInterval time.Duration) KomegaAsync {
	m.pollInterval = pollInterval
	return m
}

// intervals constructs the intervals for async assertions.
// If no timeout is set, the list will be empty.
func (m *Matcher) intervals() []interface{} {
	if m.timeout == 0 {
		return []interface{}{}
	}
	out := []interface{}{m.timeout}
	if m.pollInterval != 0 {
		out = append(out, m.pollInterval)
	}
	return out
}

// Create creates the object on the API server.
func (m *Matcher) Create(obj client.Object, opts ...client.CreateOption) gomega.GomegaAssertion {
	err := m.Client.Create(m.context(), obj, opts...)
	return gomega.Expect(err, m.extras...)
}

// Delete deletes the object from the API server.
func (m *Matcher) Delete(obj client.Object, opts ...client.DeleteOption) gomega.GomegaAssertion {
	err := m.Client.Delete(m.context(), obj, opts...)
	return gomega.Expect(err, m.extras...)
}

// Update udpates the object on the API server by fetching the object
// and applying a mutating UpdateFunc before sending the update.
func (m *Matcher) Update(obj client.Object, fn UpdateFunc, opts ...client.UpdateOption) gomega.GomegaAsyncAssertion {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	update := func() error {
		err := m.Client.Get(m.context(), key, obj)
		if err != nil {
			return err
		}
		return m.Client.Update(m.context(), fn(obj), opts...)
	}
	return gomega.Eventually(update, m.intervals()...)
}

// UpdateStatus udpates the object's status subresource on the API server by
// fetching the object and applying a mutating UpdateFunc before sending the
// update.
func (m *Matcher) UpdateStatus(obj client.Object, fn UpdateFunc, opts ...client.UpdateOption) gomega.GomegaAsyncAssertion {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	update := func() error {
		err := m.Client.Get(m.context(), key, obj)
		if err != nil {
			return err
		}
		return m.Client.Status().Update(m.context(), fn(obj), opts...)
	}
	return gomega.Eventually(update, m.intervals()...)
}

// Get gets the object from the API server.
func (m *Matcher) Get(obj client.Object) gomega.GomegaAsyncAssertion {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	get := func() error {
		return m.Client.Get(m.context(), key, obj)
	}
	return gomega.Eventually(get, m.intervals()...)
}

// List gets the list object from the API server.
func (m *Matcher) List(obj client.ObjectList, opts ...client.ListOption) gomega.GomegaAsyncAssertion {
	list := func() error {
		return m.Client.List(m.context(), obj, opts...)
	}
	return gomega.Eventually(list, m.intervals()...)
}

// Consistently continually gets the object from the API for comparison.
// It can be used to check for either List types or regular Objects.
func (m *Matcher) Consistently(obj runtime.Object, opts ...client.ListOption) gomega.GomegaAsyncAssertion {
	// If the object is a list, return a list
	if o, ok := obj.(client.ObjectList); ok {
		return m.consistentlyList(o, opts...)
	}
	if o, ok := obj.(client.Object); ok {
		return m.consistentlyObject(o)
	}
	//Should not get here
	panic("Unknown object.")
}

// consistentlyclient.Object gets an individual object from the API server.
func (m *Matcher) consistentlyObject(obj client.Object) gomega.GomegaAsyncAssertion {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	get := func() client.Object {
		err := m.Client.Get(m.context(), key, obj)
		if err != nil {
			panic(err)
		}
		return obj
	}
	return gomega.Consistently(get, m.intervals()...)
}

// consistentlyList gets an list of objects from the API server.
func (m *Matcher) consistentlyList(obj client.ObjectList, opts ...client.ListOption) gomega.GomegaAsyncAssertion {
	list := func() client.ObjectList {
		err := m.Client.List(m.context(), obj, opts...)
		if err != nil {
			panic(err)
		}
		return obj
	}
	return gomega.Consistently(list, m.intervals()...)
}

// Eventually continually gets the object from the API for comparison.
// It can be used to check for either List types or regular Objects.
func (m *Matcher) Eventually(obj runtime.Object, opts ...client.ListOption) gomega.GomegaAsyncAssertion {
	// If the object is a list, return a list
	if o, ok := obj.(client.ObjectList); ok {
		return m.eventuallyList(o, opts...)
	}
	if o, ok := obj.(client.Object); ok {
		return m.eventuallyObject(o)
	}
	//Should not get here
	panic("Unknown object.")
}

// eventuallyObject gets an individual object from the API server.
func (m *Matcher) eventuallyObject(obj client.Object) gomega.GomegaAsyncAssertion {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	get := func() client.Object {
		err := m.Client.Get(m.context(), key, obj)
		if err != nil {
			panic(err)
		}
		return obj
	}
	return gomega.Eventually(get, m.intervals()...)
}

// eventuallyList gets a list type from the API server.
func (m *Matcher) eventuallyList(obj client.ObjectList, opts ...client.ListOption) gomega.GomegaAsyncAssertion {
	list := func() client.ObjectList {
		err := m.Client.List(m.context(), obj, opts...)
		if err != nil {
			panic(err)
		}
		return obj
	}
	return gomega.Eventually(list, m.intervals()...)
}
