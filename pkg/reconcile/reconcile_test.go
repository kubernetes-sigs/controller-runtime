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

package reconcile_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconcile", func() {
	Describe("Result", func() {
		It("IsZero should return true if empty", func() {
			var res *reconcile.Result
			Expect(res.IsZero()).To(BeTrue())
			res2 := &reconcile.Result{}
			Expect(res2.IsZero()).To(BeTrue())
			res3 := reconcile.Result{}
			Expect(res3.IsZero()).To(BeTrue())
		})

		It("IsZero should return false if Requeue is set to true", func() {
			res := reconcile.Result{Requeue: true}
			Expect(res.IsZero()).To(BeFalse())
		})

		It("IsZero should return false if RequeueAfter is set to true", func() {
			res := reconcile.Result{RequeueAfter: 1 * time.Second}
			Expect(res.IsZero()).To(BeFalse())
		})
	})

	Describe("Func", func() {
		It("should call the function with the request and return a nil error.", func() {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "foo", Namespace: "bar"},
			}
			result := reconcile.Result{
				Requeue: true,
			}

			instance := reconcile.Func(func(_ context.Context, r reconcile.Request) (reconcile.Result, error) {
				defer GinkgoRecover()
				Expect(r).To(Equal(request))

				return result, nil
			})
			actualResult, actualErr := instance.Reconcile(context.Background(), request)
			Expect(actualResult).To(Equal(result))
			Expect(actualErr).NotTo(HaveOccurred())
		})

		It("should call the function with the request and return an error.", func() {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "foo", Namespace: "bar"},
			}
			result := reconcile.Result{
				Requeue: false,
			}
			err := fmt.Errorf("hello world")

			instance := reconcile.Func(func(_ context.Context, r reconcile.Request) (reconcile.Result, error) {
				defer GinkgoRecover()
				Expect(r).To(Equal(request))

				return result, err
			})
			actualResult, actualErr := instance.Reconcile(context.Background(), request)
			Expect(actualResult).To(Equal(result))
			Expect(actualErr).To(Equal(err))
		})
	})

	Describe("RequestSet", func() {
		newRequest := func(ns, name string) reconcile.Request {
			return reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      name,
				},
			}
		}

		Context("Insert", func() {
			var rs *reconcile.RequestSet

			BeforeEach(func() {
				rs = reconcile.NewRequestSet()
			})

			It("should contain request after inserting", func() {
				rs.Insert(newRequest("test", "dummy1"))

				rsList := rs.List()
				Expect(len(rsList)).To(Equal(1))
				Expect(rsList[0].Namespace).To(Equal("test"))
				Expect(rsList[0].Name).To(Equal("dummy1"))
			})

			It("should deduplicate requests when same requests are inserted repeatedly", func() {
				rs.Insert(newRequest("test", "dummy1"))
				rs.Insert(newRequest("test", "dummy1"))

				rsList := rs.List()
				Expect(len(rsList)).To(Equal(1))
				Expect(rsList[0].Namespace).To(Equal("test"))
				Expect(rsList[0].Name).To(Equal("dummy1"))
			})
		})

		Context("Delete", func() {
			var rs *reconcile.RequestSet

			BeforeEach(func() {
				rs = reconcile.NewRequestSet(
					newRequest("test", "dummy1"),
					newRequest("test", "dummy2"),
				)
			})

			It("should do nothing when deleting a request which is not in set", func() {
				rs.Delete(newRequest("test", "dummy3"))
				Expect(len(rs.List())).To(Equal(2))
			})

			It("should remove the request from set after deleting", func() {
				rs.Delete(newRequest("test", "dummy1"))

				rsList := rs.List()
				Expect(len(rsList)).To(Equal(1))
				Expect(rsList[0].Namespace).To(Equal("test"))
				Expect(rsList[0].Name).To(Equal("dummy2"))
			})
		})

		Context("List", func() {
			var rs *reconcile.RequestSet

			BeforeEach(func() {
				rs = reconcile.NewRequestSet()
			})

			It("should return an empty slice when set contains no requests", func() {
				rsList := rs.List()
				Expect(len(rsList)).To(Equal(0))
			})

			It("should return an ordered slice with all requests from set", func() {
				rs.Insert(newRequest("test", "dummy1"))
				rs.Insert(newRequest("test", "dummy2"))

				rsList := rs.List()
				Expect(len(rsList)).To(Equal(2))
				Expect(rsList[0].Namespace).To(Equal("test"))
				Expect(rsList[0].Name).To(Or(Equal("dummy1"), Equal("dummy2")))
				Expect(rsList[1].Namespace).To(Equal("test"))
				Expect(rsList[1].Name).To(Or(Equal("dummy1"), Equal("dummy2")))
			})

			It("should keep ordered if same requests are inserted into set repeatedly", func() {
				rs.Insert(newRequest("test", "dummy1"))
				rs.Insert(newRequest("test", "dummy2"))
				rs.Insert(newRequest("test", "dummy1"))
				rs.Insert(newRequest("test", "dummy2"))

				rsList := rs.List()
				Expect(len(rsList)).To(Equal(2))
				Expect(rsList[0].Namespace).To(Equal("test"))
				Expect(rsList[0].Name).To(Equal("dummy1"))
				Expect(rsList[1].Namespace).To(Equal("test"))
				Expect(rsList[1].Name).To(Equal("dummy2"))
			})
		})
	})
})
