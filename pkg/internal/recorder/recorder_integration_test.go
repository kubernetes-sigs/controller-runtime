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

package recorder_test

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("recorder", func() {
	Describe("deprecated recorder", func() {
		It("should publish events", func(ctx SpecContext) {
			By("Creating the Manager")
			cm, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating the Controller")
			recorder := cm.GetEventRecorderFor("test-deprecated-recorder") //nolint:staticcheck
			instance, err := controller.New("foo-controller", cm, controller.Options{
				Reconciler: reconcile.Func(
					func(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
						dp, err := clientset.AppsV1().Deployments(request.Namespace).Get(ctx, request.Name, metav1.GetOptions{})
						Expect(err).NotTo(HaveOccurred())
						recorder.Event(dp, corev1.EventTypeNormal, "test-reason", "test-msg")
						return reconcile.Result{}, nil
					}),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Watching Resources")
			err = instance.Watch(source.Kind(cm.GetCache(), &appsv1.Deployment{}, &handler.TypedEnqueueRequestForObject[*appsv1.Deployment]{}))
			Expect(err).NotTo(HaveOccurred())

			By("Starting the Manager")
			go func() {
				defer GinkgoRecover()
				Expect(cm.Start(ctx)).NotTo(HaveOccurred())
			}()

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "deprecated-deployment-name"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			}

			By("Invoking Reconciling")
			deployment, err = clientset.AppsV1().Deployments("default").Create(ctx, deployment, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Validate event is published as expected")
			evtWatcher, err := clientset.CoreV1().Events("default").Watch(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			resultEvent := <-evtWatcher.ResultChan()

			Expect(resultEvent.Type).To(Equal(watch.Added))
			evt, isEvent := resultEvent.Object.(*corev1.Event)
			Expect(isEvent).To(BeTrue())

			dpRef, err := ref.GetReference(scheme.Scheme, deployment)
			Expect(err).NotTo(HaveOccurred())

			Expect(evt.InvolvedObject).To(Equal(*dpRef))
			Expect(evt.Type).To(Equal(corev1.EventTypeNormal))
			Expect(evt.Reason).To(Equal("test-reason"))
			Expect(evt.Message).To(Equal("test-msg"))
		})
	})

	Describe("recorder", func() {
		It("should publish events", func(ctx SpecContext) {
			By("Creating the Manager")
			// this test needs its own env for now to not interfere with the previous one.
			// Once the deprecated API is removed this can be removed.
			testenv := &envtest.Environment{}

			cfg, err := testenv.Start()
			Expect(err).NotTo(HaveOccurred())
			defer testenv.Stop() //nolint:errcheck

			clientset, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			cm, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating the Controller")
			recorder := cm.GetEventRecorder("test-recorder")
			instance, err := controller.New("bar-controller", cm, controller.Options{
				Reconciler: reconcile.Func(
					func(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
						dp, err := clientset.AppsV1().Deployments(request.Namespace).Get(ctx, request.Name, metav1.GetOptions{})
						Expect(err).NotTo(HaveOccurred())
						recorder.Eventf(dp, nil, corev1.EventTypeNormal, "test-reason", "test-action", "test-msg")
						return reconcile.Result{}, nil
					}),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Watching Resources")
			err = instance.Watch(source.Kind(cm.GetCache(), &appsv1.Deployment{}, &handler.TypedEnqueueRequestForObject[*appsv1.Deployment]{}))
			Expect(err).NotTo(HaveOccurred())

			By("Starting the Manager")
			go func() {
				defer GinkgoRecover()
				Expect(cm.Start(ctx)).NotTo(HaveOccurred())
			}()

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "deployment-name"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			}

			By("Invoking Reconciling")
			deployment, err = clientset.AppsV1().Deployments("default").Create(ctx, deployment, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Validate event is published as expected")
			evtWatcher, err := clientset.EventsV1().Events("default").Watch(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			resultEvent := <-evtWatcher.ResultChan()

			Expect(resultEvent.Type).To(Equal(watch.Added))
			evt, isEvent := resultEvent.Object.(*eventsv1.Event)
			Expect(isEvent).To(BeTrue())

			dpRef, err := ref.GetReference(scheme.Scheme, deployment)
			Expect(err).NotTo(HaveOccurred())

			Expect(evt.Regarding).To(Equal(*dpRef))
			Expect(evt.Type).To(Equal(corev1.EventTypeNormal))
			Expect(evt.Reason).To(Equal("test-reason"))
			Expect(evt.Note).To(Equal("test-msg"))
		})
	})
})
