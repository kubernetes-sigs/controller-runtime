package integrationtests

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/internal/testing/integration"
)

var _ = Describe("The Testing Framework", func() {
	var controlPlane *integration.ControlPlane
	ctx := context.TODO()

	AfterEach(func() {
		Expect(controlPlane.Stop()).To(Succeed())
	})

	It("Successfully manages the control plane lifecycle", func() {
		var err error

		controlPlane = &integration.ControlPlane{}

		By("Starting all the control plane processes")
		err = controlPlane.Start()
		Expect(err).NotTo(HaveOccurred(), "Expected controlPlane to start successfully")

		apiServerURL := controlPlane.APIURL()
		etcdClientURL := controlPlane.APIServer.EtcdURL

		isEtcdListeningForClients := isSomethingListeningOnPort(etcdClientURL.Host)
		isAPIServerListening := isSomethingListeningOnPort(apiServerURL.Host)

		By("Ensuring Etcd is listening")
		Expect(isEtcdListeningForClients()).To(BeTrue(),
			fmt.Sprintf("Expected Etcd to listen for clients on %s,", etcdClientURL.Host))

		By("Ensuring APIServer is listening")
		c, err := controlPlane.RESTClientConfig()
		Expect(err).NotTo(HaveOccurred())
		CheckAPIServerIsReady(c)

		By("getting a kubeclient & run it against the control plane")
		c.APIPath = "/api"
		c.ContentConfig.GroupVersion = &schema.GroupVersion{Version: "v1"}
		kubeClient, err := rest.RESTClientFor(c)
		Expect(err).NotTo(HaveOccurred())
		result := &corev1.PodList{}
		err = kubeClient.Get().
			Namespace("default").
			Resource("pods").
			VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
			Do(ctx).
			Into(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(BeEmpty())

		By("getting a kubectl & run it against the control plane")
		kubeCtl := controlPlane.KubeCtl()
		stdout, stderr, err := kubeCtl.Run("get", "pods")
		Expect(err).NotTo(HaveOccurred())
		bytes, err := ioutil.ReadAll(stdout)
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes).To(BeEmpty())
		Expect(stderr).To(ContainSubstring("No resources found"))

		By("Stopping all the control plane processes")
		err = controlPlane.Stop()
		Expect(err).NotTo(HaveOccurred(), "Expected controlPlane to stop successfully")

		By("Ensuring Etcd is not listening anymore")
		Expect(isEtcdListeningForClients()).To(BeFalse(), "Expected Etcd not to listen for clients anymore")

		By("Ensuring APIServer is not listening anymore")
		Expect(isAPIServerListening()).To(BeFalse(), "Expected APIServer not to listen anymore")

		By("Not erroring when stopping a stopped ControlPlane")
		Expect(func() {
			Expect(controlPlane.Stop()).To(Succeed())
		}).NotTo(Panic())
	})

	Context("when Stop() is called on the control plane", func() {
		Context("but the control plane is not started yet", func() {
			It("does not error", func() {
				controlPlane = &integration.ControlPlane{}

				stoppingTheControlPlane := func() {
					Expect(controlPlane.Stop()).To(Succeed())
				}

				Expect(stoppingTheControlPlane).NotTo(Panic())
			})
		})
	})

	Context("when the control plane is configured with its components", func() {
		It("it does not default them", func() {
			myEtcd, myAPIServer :=
				&integration.Etcd{StartTimeout: 15 * time.Second},
				&integration.APIServer{StopTimeout: 16 * time.Second}

			controlPlane = &integration.ControlPlane{
				Etcd:      myEtcd,
				APIServer: myAPIServer,
			}

			Expect(controlPlane.Start()).To(Succeed())
			Expect(controlPlane.Etcd).To(BeIdenticalTo(myEtcd))
			Expect(controlPlane.APIServer).To(BeIdenticalTo(myAPIServer))
			Expect(controlPlane.Etcd.StartTimeout).To(Equal(15 * time.Second))
			Expect(controlPlane.APIServer.StopTimeout).To(Equal(16 * time.Second))
		})
	})

	Context("when etcd already started", func() {
		It("starts the control plane successfully", func() {
			myEtcd := &integration.Etcd{}
			Expect(myEtcd.Start()).To(Succeed())

			controlPlane = &integration.ControlPlane{
				Etcd: myEtcd,
			}

			Expect(controlPlane.Start()).To(Succeed())
		})
	})

	Context("when control plane is already started", func() {
		It("can attempt to start again without errors", func() {
			controlPlane = &integration.ControlPlane{}
			Expect(controlPlane.Start()).To(Succeed())
			Expect(controlPlane.Start()).To(Succeed())
		})
	})

	Context("when control plane starts and stops", func() {
		It("can attempt to start again without errors", func() {
			controlPlane = &integration.ControlPlane{}
			Expect(controlPlane.Start()).To(Succeed())
			Expect(controlPlane.Stop()).To(Succeed())
			Expect(controlPlane.Start()).To(Succeed())
		})
	})

	Measure("It should be fast to bring up and tear down the control plane", func(b Benchmarker) {
		b.Time("lifecycle", func() {
			controlPlane = &integration.ControlPlane{}

			Expect(controlPlane.Start()).To(Succeed())
			Expect(controlPlane.Stop()).To(Succeed())
		})
	}, 10)
})

type portChecker func() bool

func isSomethingListeningOnPort(hostAndPort string) portChecker {
	return func() bool {
		conn, err := net.DialTimeout("tcp", hostAndPort, 1*time.Second)

		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
}

// CheckAPIServerIsReady checks if the APIServer is really ready and not only
// listening.
//
// While porting some tests in k/k
// (https://github.com/hoegaarden/kubernetes/blob/287fdef1bd98646bc521f4433c1009936d5cf7a2/hack/make-rules/test-cmd-util.sh#L1524-L1535)
// we found, that the APIServer was
// listening but not serving certain APIs yet.
//
// We changed the readiness detection in the PR at
// https://github.com/kubernetes-sigs/testing_frameworks/pull/48. To confirm
// this changed behaviour does what it should do, we used the same test as in
// k/k's test-cmd (see link above) and test if certain well-known known APIs
// are actually available.
func CheckAPIServerIsReady(c *rest.Config) {
	ctx := context.TODO()
	// check pods, replicationcontrollers and services
	c.APIPath = "/api"
	c.ContentConfig.GroupVersion = &schema.GroupVersion{Version: "v1"}
	kubeClient, err := rest.RESTClientFor(c)
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("pods").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("replicationcontrollers").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("services").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	// check daemonsets, deployments, replicasets and statefulsets,
	c.APIPath = "/apis"
	c.ContentConfig.GroupVersion = &schema.GroupVersion{Group: "apps", Version: "v1"}
	kubeClient, err = rest.RESTClientFor(c)
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("daemonsets").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("deployments").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("replicasets").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("statefulsets").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	// check horizontalpodautoscalers
	c.ContentConfig.GroupVersion = &schema.GroupVersion{Group: "autoscaling", Version: "v1"}
	kubeClient, err = rest.RESTClientFor(c)
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("horizontalpodautoscalers").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())

	// check jobs
	c.ContentConfig.GroupVersion = &schema.GroupVersion{Group: "batch", Version: "v1"}
	kubeClient, err = rest.RESTClientFor(c)
	Expect(err).NotTo(HaveOccurred())

	_, err = kubeClient.Get().
		Namespace("default").
		Resource("jobs").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		Do(ctx).
		Get()
	Expect(err).NotTo(HaveOccurred())
}
