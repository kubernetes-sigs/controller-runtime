package client_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/envtest"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	clientfeatures "k8s.io/client-go/features"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

var _ = Describe("WatchList", func() {
	It("should work against the kube-apiserver", func() {

		Expect(clientfeatures.FeatureGates().Enabled(clientfeatures.WatchListClient)).To(BeTrue())

		testenv = &envtest.Environment{}
		cfg, err := testenv.Start()
		Expect(err).NotTo(HaveOccurred())
		clientset, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		opts := metav1.ListOptions{}
		opts.AllowWatchBookmarks = true
		opts.SendInitialEvents = ptr.To(true)
		opts.ResourceVersionMatch = metav1.ResourceVersionMatchNotOlderThan
		w, err := clientset.CoreV1().Secrets("kube-system").Watch(ctx, opts)
		Expect(err).NotTo(HaveOccurred())
		defer w.Stop()

		receivedWatchListStreamFromTheServer := false
		func() {
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-w.ResultChan():
					if !ok {
						panic("unexpected watch close")

					}
					if event.Type == watch.Error {
						panic(fmt.Sprintf("unexpected watch event: %v", apierrors.FromObject(event.Object)))
					}
					meta, err := meta.Accessor(event.Object)
					Expect(err).NotTo(HaveOccurred())

					switch event.Type {
					case watch.Bookmark:
						if meta.GetAnnotations()[metav1.InitialEventsAnnotationKey] != "true" {
							continue
						}
						receivedWatchListStreamFromTheServer = true
						return
					case watch.Added, watch.Modified, watch.Deleted, watch.Error:
					default:
						panic(fmt.Sprintf("unexpected watch event: %v", event.Object))
					}
				}
			}
		}()

		Expect(receivedWatchListStreamFromTheServer).To(BeTrue())
	})
})
