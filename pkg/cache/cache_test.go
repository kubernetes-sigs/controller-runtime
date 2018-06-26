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

package cache_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kcorev1 "k8s.io/api/core/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kcache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var informerCache cache.Cache
var stop chan struct{}
var knownPod1 runtime.Object
var knownPod2 runtime.Object
var knownPod3 runtime.Object

func createPod(name, namespace string, restartPolicy kcorev1.RestartPolicy) runtime.Object {
	three := int64(3)
	pod := &kcorev1.Pod{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"test-label": name,
			},
		},
		Spec: kcorev1.PodSpec{
			Containers:            []kcorev1.Container{{Name: "nginx", Image: "nginx"}},
			RestartPolicy:         restartPolicy,
			ActiveDeadlineSeconds: &three,
		},
	}
	cl, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
	err = cl.Create(context.TODO(), pod)
	Expect(err).NotTo(HaveOccurred())
	return pod
}

func deletePod(pod runtime.Object) {
	cl, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
	err = cl.Delete(context.TODO(), pod)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Informer Cache", func() {

	BeforeEach(func() {
		stop = make(chan struct{})
		Expect(cfg).NotTo(BeNil())

		By("creating three pods")
		knownPod1 = createPod("test-pod-1", "test-namespace-1", kcorev1.RestartPolicyNever)
		knownPod2 = createPod("test-pod-2", "test-namespace-2", kcorev1.RestartPolicyAlways)
		knownPod3 = createPod("test-pod-3", "test-namespace-2", kcorev1.RestartPolicyOnFailure)

		By("creating the informer cache")
		var err error
		informerCache, err = cache.New(cfg, cache.Options{})
		Expect(err).NotTo(HaveOccurred())
		By("running the cache and waiting for it to sync")
		go func() {
			defer GinkgoRecover()
			Expect(informerCache.Start(stop)).ToNot(HaveOccurred())
		}()
		Expect(informerCache.WaitForCacheSync(stop)).NotTo(BeFalse())
	})

	AfterEach(func() {
		By("cleaning up created pods")
		deletePod(knownPod1)
		deletePod(knownPod2)
		deletePod(knownPod3)

		close(stop)
	})

	Describe("Cache as a Reader", func() {
		It("should be able to list objects that haven't been watched previously", func(done Done) {
			By("listing all services in the cluster")
			listObj := &kcorev1.ServiceList{}
			Expect(informerCache.List(context.Background(), nil, listObj)).NotTo(HaveOccurred())

			By("verifying that the returned list contains the Kubernetes service")
			// NB: there has to be at least the kubernetes service in the cluster
			Expect(listObj.Items).NotTo(BeEmpty())
			hasKubeService := false
			for _, svc := range listObj.Items {
				if svc.Namespace == "default" && svc.Name == "kubernetes" {
					hasKubeService = true
					break
				}
			}
			Expect(hasKubeService).To(BeTrue())
			close(done)
		})

		It("should be able to get objects that haven't been watched previously", func(done Done) {
			By("getting the Kubernetes service")
			svc := &kcorev1.Service{}
			svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
			Expect(informerCache.Get(context.Background(), svcKey, svc)).NotTo(HaveOccurred())

			By("verifying that the returned service looks reasonable")
			Expect(svc.Name).To(Equal("kubernetes"))
			Expect(svc.Namespace).To(Equal("default"))
			close(done)
		})

		It("should support filtering by labels", func(done Done) {
			By("listing pods with a particular label")
			// NB: each pod has a "test-label" equal to the pod name
			out := kcorev1.PodList{}
			Expect(informerCache.List(context.TODO(), client.InNamespace("test-namespace-2").
				MatchingLabels(map[string]string{"test-label": "test-pod-2"}), &out)).NotTo(HaveOccurred())

			By("verifying the returned pods have the correct label")
			Expect(out.Items).NotTo(BeEmpty())
			Expect(len(out.Items)).To(Equal(1))
			actual := out.Items[0]
			Expect(actual.Labels["test-label"]).To(Equal("test-pod-2"))
			close(done)
		})

		It("should be able to list objects by namespace", func(done Done) {
			By("listing pods in test-namespace-1")
			listObj := &kcorev1.PodList{}
			Expect(informerCache.List(context.Background(),
				client.InNamespace("test-namespace-1"),
				listObj)).NotTo(HaveOccurred())

			By("verifying that the returned pods are in test-namespace-1")
			Expect(listObj.Items).NotTo(BeEmpty())
			Expect(len(listObj.Items)).To(Equal(1))
			actual := listObj.Items[0]
			Expect(actual.Namespace).To(Equal("test-namespace-1"))
			close(done)
		})

		It("should deep copy the object unless told otherwise", func(done Done) {
			By("retrieving a specific pod from the cache")
			out := &kcorev1.Pod{}
			podKey := client.ObjectKey{Name: "test-pod-2", Namespace: "test-namespace-2"}
			Expect(informerCache.Get(context.Background(), podKey, out)).NotTo(HaveOccurred())

			By("verifying the retrieved pod is equal to a known pod")
			Expect(out).To(Equal(knownPod2))

			By("altering a field in the retrieved pod")
			*out.Spec.ActiveDeadlineSeconds = 4

			By("verifying the pods are no longer equal")
			Expect(out).NotTo(Equal(knownPod2))
			close(done)
		})

		It("should error out for missing objects", func(done Done) {
			By("getting a service that does not exists")
			svc := &kcorev1.Service{}
			svcKey := client.ObjectKey{Namespace: "unknown", Name: "unknown"}

			By("verifying that an error is returned")
			Expect(informerCache.Get(context.Background(), svcKey, svc)).To(HaveOccurred())
			close(done)
		})
	})

	Describe("Cache as an Informer", func() {
		It("should be able to get informer for the object", func(done Done) {
			By("getting an shared index informer for a pod")
			pod := &kcorev1.Pod{
				ObjectMeta: kmetav1.ObjectMeta{Name: "informer-obj", Namespace: "default"},
				Spec:       kcorev1.PodSpec{Containers: []kcorev1.Container{{Name: "nginx", Image: "nginx"}}},
			}
			sii, err := informerCache.GetInformer(pod)
			Expect(err).NotTo(HaveOccurred())
			Expect(sii).NotTo(BeNil())
			Expect(sii.HasSynced()).To(Equal(true))

			By("adding an event handler listening for object creation which sends the object to a channel")
			out := make(chan interface{})
			sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: func(obj interface{}) {
				out <- obj
			}})

			By("adding an object")
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cl.Create(context.TODO(), pod)).To(Succeed())

			By("verifying the object is received on the channel")
			Eventually(out).Should(Receive(Equal(pod)))
			close(done)
		})

		It("should be able to get an informer by group/version/kind", func(done Done) {
			By("getting an shared index informer for gvk = core/v1/pod")
			gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
			sii, err := informerCache.GetInformerForKind(gvk)
			Expect(err).NotTo(HaveOccurred())
			Expect(sii).NotTo(BeNil())
			Expect(sii.HasSynced()).To(Equal(true))

			By("adding an event handler listening for object creation which sends the object to a channel")
			out := make(chan interface{})
			sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: func(obj interface{}) {
				out <- obj
			}})

			By("adding an object")
			cl, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			pod := &kcorev1.Pod{
				ObjectMeta: kmetav1.ObjectMeta{Name: "informer-gvk", Namespace: "default"},
				Spec:       kcorev1.PodSpec{Containers: []kcorev1.Container{{Name: "nginx", Image: "nginx"}}},
			}
			Expect(cl.Create(context.TODO(), pod)).To(Succeed())

			By("verifying the object is received on the channel")
			Eventually(out).Should(Receive(Equal(pod)))
			close(done)
		})

		It("should be able to index an object field then retrieve objects by that field", func(done Done) {
			By("creating the cache")
			informer, err := cache.New(cfg, cache.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("indexing the restartPolicy field of the Pod object before starting")
			pod := &kcorev1.Pod{}
			Expect(informer.IndexField(pod, "spec.restartPolicy", func(obj runtime.Object) []string {
				return []string{string(obj.(*kcorev1.Pod).Spec.RestartPolicy)}
			})).ToNot(HaveOccurred())

			By("running the cache and waiting for it to sync")
			go func() {
				defer GinkgoRecover()
				Expect(informer.Start(stop)).ToNot(HaveOccurred())
			}()
			Expect(informer.WaitForCacheSync(stop)).NotTo(BeFalse())

			By("listing Pods with restartPolicyOnFailure")
			listObj := &kcorev1.PodList{}
			Expect(informer.List(context.Background(),
				client.MatchingField("spec.restartPolicy", "OnFailure"),
				listObj)).NotTo(HaveOccurred())

			By("verifying that the returned pods have correct restart policy")
			Expect(listObj.Items).NotTo(BeEmpty())
			Expect(len(listObj.Items)).To(Equal(1))
			actual := listObj.Items[0]
			Expect(actual.Name).To(Equal("test-pod-3"))
			close(done)
		})
	})
})
