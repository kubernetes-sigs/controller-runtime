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

package controllerutil_test

import (
	"context"
	"fmt"
	"math/rand"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
				{
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
				{
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

	Describe("CreateOrUpdate", func() {
		var deploy *appsv1.Deployment
		var deplSpec appsv1.DeploymentSpec
		var deplKey types.NamespacedName
		var specr controllerutil.MutateFn

		BeforeEach(func() {
			deploy = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("deploy-%d", rand.Int31()),
					Namespace: "default",
				},
			}

			deplSpec = appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"foo": "bar",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "busybox",
								Image: "busybox",
							},
						},
					},
				},
			}

			deplKey = types.NamespacedName{
				Name:      deploy.Name,
				Namespace: deploy.Namespace,
			}

			specr = deploymentSpecr(deploy, deplSpec)
		})

		It("creates a new object if one doesn't exists", func() {
			op, err := controllerutil.CreateOrUpdate(context.TODO(), c, deploy, specr)

			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultCreated")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))

			By("actually having the deployment created")
			fetched := &appsv1.Deployment{}
			Expect(c.Get(context.TODO(), deplKey, fetched)).To(Succeed())

			By("being mutated by MutateFn")
			Expect(fetched.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(fetched.Spec.Template.Spec.Containers[0].Name).To(Equal(deplSpec.Template.Spec.Containers[0].Name))
			Expect(fetched.Spec.Template.Spec.Containers[0].Image).To(Equal(deplSpec.Template.Spec.Containers[0].Image))
		})

		It("updates existing object", func() {
			var scale int32 = 2
			op, err := controllerutil.CreateOrUpdate(context.TODO(), c, deploy, specr)
			Expect(err).NotTo(HaveOccurred())
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))

			op, err = controllerutil.CreateOrUpdate(context.TODO(), c, deploy, deploymentScaler(deploy, scale))
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultUpdated")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultUpdated))

			By("actually having the deployment scaled")
			fetched := &appsv1.Deployment{}
			Expect(c.Get(context.TODO(), deplKey, fetched)).To(Succeed())
			Expect(*fetched.Spec.Replicas).To(Equal(scale))
		})

		It("updates only changed objects", func() {
			op, err := controllerutil.CreateOrUpdate(context.TODO(), c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			op, err = controllerutil.CreateOrUpdate(context.TODO(), c, deploy, deploymentIdentity)
			By("returning no error")
			Expect(err).NotTo(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
		})

		It("errors when MutateFn changes objct name on creation", func() {
			op, err := controllerutil.CreateOrUpdate(context.TODO(), c, deploy, func() error {
				Expect(specr()).To(Succeed())
				return deploymentRenamer(deploy)()
			})

			By("returning error")
			Expect(err).To(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
		})

		It("errors when MutateFn renames an object", func() {
			op, err := controllerutil.CreateOrUpdate(context.TODO(), c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			op, err = controllerutil.CreateOrUpdate(context.TODO(), c, deploy, deploymentRenamer(deploy))

			By("returning error")
			Expect(err).To(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
		})

		It("errors when object namespace changes", func() {
			op, err := controllerutil.CreateOrUpdate(context.TODO(), c, deploy, specr)

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultCreated))
			Expect(err).NotTo(HaveOccurred())

			op, err = controllerutil.CreateOrUpdate(context.TODO(), c, deploy, deploymentNamespaceChanger(deploy))

			By("returning error")
			Expect(err).To(HaveOccurred())

			By("returning OperationResultNone")
			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
		})

		It("aborts immediately if there was an error initially retrieving the object", func() {
			op, err := controllerutil.CreateOrUpdate(context.TODO(), errorReader{c}, deploy, func() error {
				Fail("Mutation method should not run")
				return nil
			})

			Expect(op).To(BeEquivalentTo(controllerutil.OperationResultNone))
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ metav1.Object = &errMetaObj{}

type errMetaObj struct {
	metav1.ObjectMeta
}

func deploymentSpecr(deploy *appsv1.Deployment, spec appsv1.DeploymentSpec) controllerutil.MutateFn {
	return func() error {
		deploy.Spec = spec
		return nil
	}
}

var deploymentIdentity controllerutil.MutateFn = func() error {
	return nil
}

func deploymentRenamer(deploy *appsv1.Deployment) controllerutil.MutateFn {
	return func() error {
		deploy.Name = fmt.Sprintf("%s-1", deploy.Name)
		return nil
	}
}

func deploymentNamespaceChanger(deploy *appsv1.Deployment) controllerutil.MutateFn {
	return func() error {
		deploy.Namespace = fmt.Sprintf("%s-1", deploy.Namespace)
		return nil

	}
}

func deploymentScaler(deploy *appsv1.Deployment, replicas int32) controllerutil.MutateFn {
	fn := func() error {
		deploy.Spec.Replicas = &replicas
		return nil
	}
	return fn
}

type errorReader struct {
	client.Client
}

func (e errorReader) Get(ctx context.Context, key client.ObjectKey, into runtime.Object) error {
	return fmt.Errorf("unexpected error")
}
