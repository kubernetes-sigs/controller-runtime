package webhook_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
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
	Context("when running a webhook server without a manager", func() {
		It("should reject create request for webhook that rejects all requests", func(done Done) {
			server := webhook.Server{
				Port:    testenv.WebhookInstallOptions.LocalServingPort,
				Host:    testenv.WebhookInstallOptions.LocalServingHost,
				CertDir: testenv.WebhookInstallOptions.LocalServingCertDir,
			}
			server.Register("/failing", &webhook.Admission{Handler: &rejectingValidator{}})

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				_ = server.StartStandalone(ctx, scheme.Scheme)
			}()

			Eventually(func() bool {
				err := c.Create(context.TODO(), obj)
				return errors.ReasonForError(err) == metav1.StatusReason("Always denied")
			}, 1*time.Second).Should(BeTrue())

			cancel()
			close(done)
		})
	})
	Context("when running a standalone webhook", func() {
		It("should reject create request for webhook that rejects all requests", func(done Done) {
			ctx, cancel := context.WithCancel(context.Background())
			// generate tls cfg
			certPath := filepath.Join(testenv.WebhookInstallOptions.LocalServingCertDir, "tls.crt")
			keyPath := filepath.Join(testenv.WebhookInstallOptions.LocalServingCertDir, "tls.key")

			certWatcher, err := certwatcher.New(certPath, keyPath)
			Expect(err).NotTo(HaveOccurred())
			go func() {
				Expect(certWatcher.Start(ctx)).NotTo(HaveOccurred())
			}()

			cfg := &tls.Config{
				NextProtos:     []string{"h2"},
				GetCertificate: certWatcher.GetCertificate,
			}

			// generate listener
			listener, err := tls.Listen("tcp",
				net.JoinHostPort(testenv.WebhookInstallOptions.LocalServingHost,
					strconv.Itoa(int(testenv.WebhookInstallOptions.LocalServingPort))), cfg)
			Expect(err).NotTo(HaveOccurred())

			// create and register the standalone webhook
			hook, err := admission.StandaloneWebhook(&webhook.Admission{Handler: &rejectingValidator{}}, admission.StandaloneOptions{})
			Expect(err).NotTo(HaveOccurred())
			http.Handle("/failing", hook)

			// run the http server
			srv := &http.Server{}
			go func() {
				idleConnsClosed := make(chan struct{})
				go func() {
					<-ctx.Done()
					Expect(srv.Shutdown(context.Background())).NotTo(HaveOccurred())
					close(idleConnsClosed)
				}()
				_ = srv.Serve(listener)
				<-idleConnsClosed
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
