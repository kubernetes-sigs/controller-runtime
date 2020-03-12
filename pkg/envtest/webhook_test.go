package envtest

import (
	"context"
	"crypto/tls"
	"fmt"
	"path/filepath"
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

var _ = Describe("Test", func() {

	Describe("Webhook", func() {
		It("should reject create request for webhook that rejects all requests", func(done Done) {
			m, err := manager.New(env.Config, manager.Options{
				Port:    env.WebhookInstallOptions.LocalServingPort,
				Host:    env.WebhookInstallOptions.LocalServingHost,
				CertDir: env.WebhookInstallOptions.LocalServingCertDir,
			}) // we need manager here just to leverage manager.SetFields
			Expect(err).NotTo(HaveOccurred())
			server := m.GetWebhookServer()
			server.Register("/failing", &webhook.Admission{Handler: &rejectingValidator{}})

			stopCh := make(chan struct{})
			go func() {
				_ = server.Start(stopCh)
			}()

			c, err := client.New(env.Config, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			obj := &appsv1.Deployment{
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

			Eventually(func() bool {
				err = c.Create(context.TODO(), obj)
				return errors.ReasonForError(err) == metav1.StatusReason("Always denied")
			}, 1*time.Second).Should(BeTrue())

			close(stopCh)
			close(done)
		})
	})

	Describe("Webhook With Custom TLS", func() {
		It("should reject requests due to bad certificate", func(done Done) {
			_, err := env.WebhookInstallOptions.setupCA()
			dir := env.WebhookInstallOptions.LocalServingCertDir
			certFile := filepath.Join(dir, "tls.crt")
			keyFile := filepath.Join(dir, "tls.key")
			Expect(err).NotTo(HaveOccurred())
			sCert, err := tls.LoadX509KeyPair(certFile, keyFile)
			m, err := manager.New(env.Config, manager.Options{
				Port: env.WebhookInstallOptions.LocalServingPort,
				Host: env.WebhookInstallOptions.LocalServingHost,
				// CertDir: env.WebhookInstallOptions.LocalServingCertDir,
				WebhookTLSConfig: &tls.Config{
					NextProtos:   []string{"h2"},
					Certificates: []tls.Certificate{sCert},
					ClientAuth:   tls.NoClientCert,
				},
			}) // we need manager here just to leverage manager.SetFields
			Expect(err).NotTo(HaveOccurred())
			server := m.GetWebhookServer()
			server.Register("/failing", &webhook.Admission{Handler: &rejectingValidator{}})

			stopCh := make(chan struct{})
			go func() {
				_ = server.Start(stopCh)
			}()

			c, err := client.New(env.Config, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			obj := &appsv1.Deployment{
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

			Eventually(func() bool {
				err = c.Create(context.TODO(), obj)
				// Bad Certificate shows up as InternalError
				return errors.ReasonForError(err) == metav1.StatusReason("InternalError")
			}, 1*time.Second).Should(BeTrue())

			close(stopCh)
			close(done)
		})
	})
})

type rejectingValidator struct {
}

func (v *rejectingValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	return admission.Denied(fmt.Sprint("Always denied"))
}
