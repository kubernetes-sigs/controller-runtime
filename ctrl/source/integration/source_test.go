package integration

import (
	"fmt"

	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/event"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/eventhandler"
	"github.com/kubernetes-sigs/kubebuilder/pkg/ctrl/source"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
)

var _ = Describe("Source", func() {
	var instance1, instance2 source.KindSource
	var obj runtime.Object
	var q workqueue.RateLimitingInterface
	var c1, c2 chan string
	var ns string
	count := 0

	BeforeEach(func() {
		// Create the namespace for the test
		ns = fmt.Sprintf("ctrl-source-kindsource-%v", count)
		count++
		_, err := clientset.CoreV1().Namespaces().Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})
		Expect(err).NotTo(HaveOccurred())

		q = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test")
		c1 = make(chan string)
		c2 = make(chan string)
	})

	JustBeforeEach(func() {
		instance1 = source.KindSource{Type: obj}
		instance1.InitInformerCache(icache)

		instance2 = source.KindSource{Type: obj}
		instance2.InitInformerCache(icache)
	})

	AfterEach(func() {
		err := clientset.CoreV1().Namespaces().Delete(ns, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
		close(c1)
		close(c2)
	})

	Describe("KindSource", func() {
		Context("for a Deployment resource", func() {
			obj = &appsv1.Deployment{}

			It("should provide Deployment Events", func(done Done) {
				var created, updated, deleted *appsv1.Deployment
				var err error

				// Get the client and Deployment used to create events
				client := clientset.AppsV1().Deployments(ns)
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

				// Create an event handler to verify the events
				ehf := func(c chan string) eventhandler.EventHandlerFuncs {
					return eventhandler.EventHandlerFuncs{
						CreateFunc: func(rli workqueue.RateLimitingInterface, evt event.CreateEvent) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))

							// Compare meta and objects
							<-c
							m, ok := evt.Meta.(*metav1.ObjectMeta)
							Expect(ok).To(BeTrue(), fmt.Sprintf(
								"expect %T to be %T", evt.Meta, &metav1.ObjectMeta{}))
							Expect(m).To(Equal(&created.ObjectMeta))
							Expect(evt.Object).To(Equal(created))
							c <- "create"
						},
						UpdateFunc: func(rli workqueue.RateLimitingInterface, evt event.UpdateEvent) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))

							// Compare new meta and objects
							<-c
							m, ok := evt.MetaNew.(*metav1.ObjectMeta)
							Expect(ok).To(BeTrue(), fmt.Sprintf(
								"expect %T to be %T", evt.MetaNew, &metav1.ObjectMeta{}))
							Expect(m).To(Equal(&updated.ObjectMeta))
							Expect(evt.ObjectNew).To(Equal(updated))

							// Compare old meta and objects
							m, ok = evt.MetaOld.(*metav1.ObjectMeta)
							Expect(ok).To(BeTrue(), fmt.Sprintf(
								"expect %T to be %T", evt.MetaOld, &metav1.ObjectMeta{}))
							Expect(m).To(Equal(&created.ObjectMeta))
							Expect(evt.ObjectOld).To(Equal(created))
							c <- "update"
						},
						DeleteFunc: func(rli workqueue.RateLimitingInterface, evt event.DeleteEvent) {
							defer GinkgoRecover()
							Expect(rli).To(Equal(q))

							// Compare meta and objects
							<-c
							deleted.SetResourceVersion("")
							evt.Meta.SetResourceVersion("")

							m, ok := evt.Meta.(*metav1.ObjectMeta)
							Expect(ok).To(BeTrue(), fmt.Sprintf(
								"expect %T to be %T", evt.Meta, &metav1.ObjectMeta{}))
							Expect(m).To(Equal(&deleted.ObjectMeta))
							Expect(evt.Object).To(Equal(deleted))
							c <- "delete"
						},
					}
				}
				eh1 := ehf(c1)
				eh2 := ehf(c2)

				// Create 2 instances
				instance1.Start(eh1, q)
				instance2.Start(eh2, q)

				By("Creating a Deployment and expecting the CreateEvent.")
				created, err = client.Create(deployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(created).NotTo(BeNil())
				c1 <- ""
				c2 <- ""
				Expect(<-c1).To(Equal("create"))
				Expect(<-c2).To(Equal("create"))

				By("Updating a Deployment and expecting the UpdateEvent.")
				updated = created.DeepCopy()
				fmt.Printf("\n\nO1 %v\n", updated)
				updated.Labels = map[string]string{"biz": "buz"}
				updated, err = client.Update(updated)
				fmt.Printf("\n\nO2 %v\n", updated)
				Expect(err).NotTo(HaveOccurred())
				c1 <- ""
				c2 <- ""
				Expect(<-c1).To(Equal("update"))
				Expect(<-c2).To(Equal("update"))

				By("Deleting a Deployment and expecting the Delete.")
				deleted = updated.DeepCopy()
				err = client.Delete(created.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
				c1 <- ""
				c2 <- ""
				Expect(<-c1).To(Equal("delete"))
				Expect(<-c2).To(Equal("delete"))

				close(done)
			}, 5)
		})

		// TODO: Write this test
		Context("for a Foo CRD resource", func() {
			It("should provide Foo Events", func() {

			})
		})
	})
})
