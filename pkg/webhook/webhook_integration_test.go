package webhook_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("Webhook", func() {
	var c client.Client
	var obj *appsv1.Deployment
	BeforeEach(func() {
		Expect(cfg).NotTo(BeNil())
		var err error
		c, err = client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())

		obj = &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "default",
			},
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
	})
	Context("when running a webhook server with a manager", func() {
		It("should reject create request for webhook that rejects all requests", func(done Done) {
			m, err := manager.New(cfg, manager.Options{
				Port:    testenv.WebhookInstallOptions.LocalServingPort,
				Host:    testenv.WebhookInstallOptions.LocalServingHost,
				CertDir: testenv.WebhookInstallOptions.LocalServingCertDir,
			}) // we need manager here just to leverage manager.SetFields
			Expect(err).NotTo(HaveOccurred())
			server := m.GetWebhookServer()
			server.Register("/failing", &webhook.Admission{Handler: &rejectingValidator{}})

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				_ = server.Start(ctx)
			}()

			Eventually(func() bool {
				err = c.Create(context.TODO(), obj)
				return errors.ReasonForError(err) == metav1.StatusReason("Always denied")
			}, 1*time.Second).Should(BeTrue())

			cancel()
			close(done)
		})
	})
	Context("when running a webhook server without a manager ", func() {
		It("should reject create request for webhook that rejects all requests", func(done Done) {
			opts := webhook.Options{
				Port:    testenv.WebhookInstallOptions.LocalServingPort,
				Host:    testenv.WebhookInstallOptions.LocalServingHost,
				CertDir: testenv.WebhookInstallOptions.LocalServingCertDir,
			}
			server, err := webhook.NewUnmanaged(opts)
			Expect(err).NotTo(HaveOccurred())
			server.Register("/failing", &webhook.Admission{Handler: &rejectingValidator{}})

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				_ = server.Start(ctx)
			}()

			Eventually(func() bool {
				err = c.Create(context.TODO(), obj)
				return errors.ReasonForError(err) == metav1.StatusReason("Always denied")
			}, 1*time.Second).Should(BeTrue())

			cancel()
			close(done)
		})
	})
})

type rejectingValidator struct {
}

func (v *rejectingValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	return admission.Denied(fmt.Sprint("Always denied"))
}
