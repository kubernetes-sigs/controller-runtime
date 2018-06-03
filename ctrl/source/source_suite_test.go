package source_test

import (
	"testing"
	"time"

	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"github.com/kubernetes-sigs/kubebuilder/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Source Suite", []Reporter{test.NewlineReporter{}})
}

var testenv *test.TestEnvironment
var config *rest.Config
var clientset *kubernetes.Clientset
var icache informer.IndexInformerCache
var stop = make(chan struct{})

var _ = BeforeSuite(func() {
	testenv = &test.TestEnvironment{}

	var err error
	config, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	time.Sleep(1 * time.Second)

	clientset, err = kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	icache = &informer.IndexedCache{Config: config}
	icache.InformerFor(&appsv1.Deployment{})
	icache.InformerFor(&appsv1.ReplicaSet{})
	err = icache.Start(stop)
})

var _ = AfterSuite(func() {
	close(stop)
	testenv.Stop()
})
