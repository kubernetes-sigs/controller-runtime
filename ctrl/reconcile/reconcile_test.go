package reconcile_test

import (
	"fmt"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/reconcile"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Reconcile", func() {
	Describe("DefaultReconcileFunc", func() {
		It(" should return a nil error.", func() {
			reconcile.DefaultReconcileFunc(reconcile.ReconcileRequest{})
		})
	})

	Describe("ReconcileFunc", func() {
		It("should call the function with the request and return a nil error.", func() {
			request := reconcile.ReconcileRequest{
				NamespacedName: types.NamespacedName{Name: "foo", Namespace: "bar"},
			}
			result := reconcile.ReconcileResult{
				Requeue: true,
			}

			instance := reconcile.ReconcileFunc(func(r reconcile.ReconcileRequest) (reconcile.ReconcileResult, error) {
				defer GinkgoRecover()
				Expect(r).To(Equal(request))

				return result, nil
			})
			actualResult, actualErr := instance(request)
			Expect(actualResult).To(Equal(result))
			Expect(actualErr).NotTo(HaveOccurred())
		})

		It("should call the function with the request and return an error.", func() {
			request := reconcile.ReconcileRequest{
				NamespacedName: types.NamespacedName{Name: "foo", Namespace: "bar"},
			}
			result := reconcile.ReconcileResult{
				Requeue: false,
			}
			err := fmt.Errorf("hello world")

			instance := reconcile.ReconcileFunc(func(r reconcile.ReconcileRequest) (reconcile.ReconcileResult, error) {
				defer GinkgoRecover()
				Expect(r).To(Equal(request))

				return result, err
			})
			actualResult, actualErr := instance(request)
			Expect(actualResult).To(Equal(result))
			Expect(actualErr).To(Equal(err))
		})
	})
})
