package controllerutil_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Controllerutil", func() {
	Describe("SetControllerReference", func() {
		It("should set the OwnerReference if it can find the group version kind", func() {
			rs := &appsv1.ReplicaSet{}
			dep := &extensionsv1beta1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", UID: "foo-uid"},
			}

			Expect(controllerutil.SetControllerReference(dep, rs, scheme.Scheme)).NotTo(HaveOccurred())
			t := true
			Expect(rs.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
				Name:               "foo",
				Kind:               "Deployment",
				APIVersion:         "extensions/v1beta1",
				UID:                "foo-uid",
				Controller:         &t,
				BlockOwnerDeletion: &t,
			}))
		})

		It("should return an error if it can't find the group version kind of the owner", func() {
			rs := &appsv1.ReplicaSet{}
			dep := &extensionsv1beta1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
			}
			Expect(controllerutil.SetControllerReference(dep, rs, runtime.NewScheme())).To(HaveOccurred())
		})

		It("should return an error if the owner isn't a runtime.Object", func() {
			rs := &appsv1.ReplicaSet{}
			Expect(controllerutil.SetControllerReference(&errMetaObj{}, rs, scheme.Scheme)).To(HaveOccurred())
		})

		It("should return an error if object is already owned by another controller", func() {
			t := true
			rsOwners := []metav1.OwnerReference{
				metav1.OwnerReference{
					Name:               "bar",
					Kind:               "Deployment",
					APIVersion:         "extensions/v1beta1",
					UID:                "bar-uid",
					Controller:         &t,
					BlockOwnerDeletion: &t,
				},
			}
			rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", OwnerReferences: rsOwners}}
			dep := &extensionsv1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", UID: "foo-uid"}}

			err := controllerutil.SetControllerReference(dep, rs, scheme.Scheme)

			Expect(err).To(HaveOccurred())
			Expect(err).To(BeAssignableToTypeOf(&controllerutil.AlreadyOwnedError{}))
		})

		It("should not duplicate existing owner reference", func() {
			f := false
			t := true
			rsOwners := []metav1.OwnerReference{
				metav1.OwnerReference{
					Name:               "foo",
					Kind:               "Deployment",
					APIVersion:         "extensions/v1beta1",
					UID:                "foo-uid",
					Controller:         &f,
					BlockOwnerDeletion: &t,
				},
			}
			rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", OwnerReferences: rsOwners}}
			dep := &extensionsv1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", UID: "foo-uid"}}

			Expect(controllerutil.SetControllerReference(dep, rs, scheme.Scheme)).NotTo(HaveOccurred())
			Expect(rs.OwnerReferences).To(ConsistOf(metav1.OwnerReference{
				Name:               "foo",
				Kind:               "Deployment",
				APIVersion:         "extensions/v1beta1",
				UID:                "foo-uid",
				Controller:         &t,
				BlockOwnerDeletion: &t,
			}))
		})
	})
})

var _ metav1.Object = &errMetaObj{}

type errMetaObj struct {
	metav1.ObjectMeta
}
