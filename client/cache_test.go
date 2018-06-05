package client_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	kapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	. "github.com/kubernetes-sigs/kubebuilder/pkg/client"
)

var _ = Describe("Indexers", func() {
	three := int64(3)
	knownPodKey := ObjectKey{Name: "some-pod", Namespace: "some-ns"}
	knownPod3Key := ObjectKey{Name: "some-pod", Namespace: "some-other-ns"}
	knownVolumeKey := ObjectKey{Name: "some-vol", Namespace: "some-ns"}
	knownPod := &kapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: knownPodKey.Name,
			Namespace: knownPodKey.Namespace,
		},
		Spec: kapi.PodSpec{
			RestartPolicy: kapi.RestartPolicyNever,
			ActiveDeadlineSeconds: &three,
		},
	}
	knownPod2 := &kapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: knownVolumeKey.Name,
			Namespace: knownVolumeKey.Namespace,
			Labels: map[string]string{
				"somelbl": "someval",
			},
		},
		Spec: kapi.PodSpec{
			RestartPolicy: kapi.RestartPolicyAlways,
		},
	}
	knownPod3 := &kapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: knownPod3Key.Name,
			Namespace: knownPod3Key.Namespace,
			Labels: map[string]string{
				"somelbl": "someval",
			},
		},
		Spec: kapi.PodSpec{
			RestartPolicy: kapi.RestartPolicyNever,
		},
	}
	knownVolume := &kapi.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: knownVolumeKey.Name,
			Namespace: knownVolumeKey.Namespace,
		},
	}
	var multiCache *ObjectCache

	BeforeEach(func() {
		multiCache = NewObjectCache()
		podIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})
		volumeIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})
		IndexByField(podIndexer, "spec.restartPolicy", func(obj runtime.Object) []string {
			return []string{string(obj.(*kapi.Pod).Spec.RestartPolicy)}
		})
		Expect(podIndexer.Add(knownPod)).NotTo(HaveOccurred())
		Expect(podIndexer.Add(knownPod2)).NotTo(HaveOccurred())
		Expect(podIndexer.Add(knownPod3)).NotTo(HaveOccurred())
		Expect(volumeIndexer.Add(knownVolume)).NotTo(HaveOccurred())
		multiCache.RegisterCache(&kapi.Pod{}, kapi.SchemeGroupVersion.WithKind("Pod"), podIndexer)
		multiCache.RegisterCache(&kapi.PersistentVolume{}, kapi.SchemeGroupVersion.WithKind("PersistentVolume"), volumeIndexer)
	})

	Describe("Client interface wrapper around an indexer", func() {
		var singleCache ReadInterface

		BeforeEach(func() {
			var ok bool
			singleCache, ok = multiCache.CacheFor(&kapi.Pod{})
			Expect(ok).To(BeTrue())
		})

		It("should be able to fetch a particular object by key", func() {
			out := kapi.Pod{}
			Expect(singleCache.Get(context.TODO(), knownPodKey, &out)).NotTo(HaveOccurred())
			Expect(&out).To(Equal(knownPod))
		})

		It("should error out for missing objects", func() {
			Expect(singleCache.Get(context.TODO(), ObjectKey{Name: "unkown-pod"}, &kapi.Pod{})).To(HaveOccurred())
		})

		It("should be able to list objects by namespace", func() {
			out := kapi.PodList{}
			Expect(singleCache.List(context.TODO(), InNamespace(knownPodKey.Namespace), &out)).NotTo(HaveOccurred())
			Expect(out.Items).To(ConsistOf(*knownPod, *knownPod2))
		})

		It("should error out if the incorrect object type is passed for this indexer", func() {
			Expect(singleCache.Get(context.TODO(), knownPodKey, &kapi.PersistentVolume{})).To(HaveOccurred())
		})

		It("should deep copy the object unless told otherwise", func() {
			out := kapi.Pod{}
			Expect(singleCache.Get(context.TODO(), knownPodKey, &out)).NotTo(HaveOccurred())
			Expect(&out).To(Equal(knownPod))

			*out.Spec.ActiveDeadlineSeconds = 4
			Expect(*out.Spec.ActiveDeadlineSeconds).NotTo(Equal(*knownPod.Spec.ActiveDeadlineSeconds))
		})

		It("should support filtering by labels", func() {
			out := kapi.PodList{}
			Expect(singleCache.List(context.TODO(), InNamespace(knownPodKey.Namespace).MatchingLabels(map[string]string{"somelbl": "someval"}), &out)).NotTo(HaveOccurred())
			Expect(out.Items).To(ConsistOf(*knownPod2))
		})

		It("should support filtering by a single field=value specification, if previously indexed", func() {
			By("listing by field selector in a namespace")
			out := kapi.PodList{}
			Expect(singleCache.List(context.TODO(), InNamespace(knownPodKey.Namespace).MatchingField("spec.restartPolicy", "Always"), &out)).NotTo(HaveOccurred())
			Expect(out.Items).To(ConsistOf(*knownPod2))

			By("listing by field selector across all namespaces")
			Expect(singleCache.List(context.TODO(), MatchingField("spec.restartPolicy", "Never"), &out)).NotTo(HaveOccurred())
			Expect(out.Items).To(ConsistOf(*knownPod, *knownPod3))
		})
	})

	Describe("Client interface wrapper around multiple indexers", func() {
		It("should be able to fetch any known object by key and type", func() {
			outPod := kapi.Pod{}
			Expect(multiCache.Get(context.TODO(), knownPodKey, &outPod)).NotTo(HaveOccurred())
			Expect(&outPod).To(Equal(knownPod))

			outVol := kapi.PersistentVolume{}
			Expect(multiCache.Get(context.TODO(), knownVolumeKey, &outVol)).NotTo(HaveOccurred())
			Expect(&outVol).To(Equal(knownVolume))
		})

		It("should error out if the object type is unknown", func() {
			Expect(multiCache.Get(context.TODO(), knownPodKey, &kapi.PersistentVolumeClaim{})).To(HaveOccurred())
		})

		It("should deep copy the object unless told otherwise", func() {
			out := kapi.Pod{}
			Expect(multiCache.Get(context.TODO(), knownPodKey, &out)).NotTo(HaveOccurred())
			Expect(&out).To(Equal(knownPod))

			*out.Spec.ActiveDeadlineSeconds = 4
			Expect(*out.Spec.ActiveDeadlineSeconds).NotTo(Equal(*knownPod.Spec.ActiveDeadlineSeconds))
		})

		It("should be able to fetch single caches for known types", func () {
			indexer, ok := multiCache.CacheFor(&kapi.Pod{})
			Expect(ok).To(BeTrue())
			Expect(indexer).NotTo(BeNil())
			
			_, ok2 := multiCache.CacheFor(&kapi.PersistentVolumeClaim{})
			Expect(ok2).To(BeFalse())
		})
	})
})
