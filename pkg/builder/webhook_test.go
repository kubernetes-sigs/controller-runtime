/*
Copyright 2019 The Kubernetes Authors.

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

package builder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	admissionReviewGV = `{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/`

	svcBaseAddr = "http://svc-name.svc-ns.svc"

	customPath = "/custom-path"

	userAgentHeader             = "User-Agent"
	userAgentCtxKey agentCtxKey = "UserAgent"
	userAgentValue              = "test"
)

type agentCtxKey string

var _ = Describe("webhook", func() {
	Describe("New", func() {
		Context("v1 AdmissionReview", func() {
			runTests("v1")
		})
		Context("v1beta1 AdmissionReview", func() {
			runTests("v1beta1")
		})
	})
})

func runTests(admissionReviewVersion string) {
	var (
		stop          chan struct{}
		logBuffer     *gbytes.Buffer
		testingLogger logr.Logger
	)

	BeforeEach(func() {
		stop = make(chan struct{})
		logBuffer = gbytes.NewBuffer()
		testingLogger = zap.New(zap.JSONEncoder(), zap.WriteTo(io.MultiWriter(logBuffer, GinkgoWriter)))
	})

	AfterEach(func() {
		close(stop)
	})

	DescribeTable("scaffold a defaulting webhook",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestDefaulterObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
			builder.Register(&TestDefaulterObject{}, &TestDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhookBuilder := WebhookManagedBy(m, &TestDefaulterObject{})
			build(webhookBuilder)
			err = webhookBuilder.
				WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
					return admission.DefaultLogConstructor(testingLogger, req)
				}).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"foo.test.org",
      "version":"v1",
      "kind":"TestDefaulterObject"
    },
    "resource":{
      "group":"foo.test.org",
      "version":"v1",
      "resource":"testdefaulter"
    },
    "namespace":"default",
    "name":"foo",
    "operation":"CREATE",
    "object":{
      "replica":1
    },
    "oldObject":null
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testDefaulterGVK)
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable fields")
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"allowed":true`))
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"patch":`))
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"code":200`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Defaulting object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testdefaulter"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))

			By("sending a request to a validating webhook path that doesn't exist")
			path = generateValidatePath(testDefaulterGVK)
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))
		},
		Entry("Custom Defaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithCustomDefaulter(&TestCustomDefaulter{})
		}),
		Entry("Defaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithDefaulter(&testDefaulter{})
		}),
	)

	DescribeTable("should scaffold a custom defaulting webhook with a custom path",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestDefaulterObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
			builder.Register(&TestDefaulterObject{}, &TestDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			customPath := "/custom-defaulting-path"
			webhookBuilder := WebhookManagedBy(m, &TestDefaulterObject{})
			build(webhookBuilder)
			err = webhookBuilder.
				WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
					return admission.DefaultLogConstructor(testingLogger, req)
				}).
				WithDefaulterCustomPath(customPath).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"foo.test.org",
      "version":"v1",
      "kind":"TestDefaulterObject"
    },
    "resource":{
      "group":"foo.test.org",
      "version":"v1",
      "resource":"testdefaulter"
    },
    "namespace":"default",
    "name":"foo",
    "operation":"CREATE",
    "object":{
      "replica":1
    },
    "oldObject":null
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path that have been overriten by a custom path")
			path, err := generateCustomPath(customPath)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable fields")
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"allowed":true`))
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"patch":`))
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"code":200`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Defaulting object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testdefaulter"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))

			By("sending a request to a mutating webhook path")
			path = generateMutatePath(testDefaulterGVK)
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))
		},
		Entry("Custom Defaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithCustomDefaulter(&TestCustomDefaulter{})
		}),
		Entry("Defaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithDefaulter(&testDefaulter{})
		}),
	)

	DescribeTable("should scaffold a custom defaulting webhook which recovers from panics",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestDefaulterObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
			builder.Register(&TestDefaulterObject{}, &TestDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhookBuilder := WebhookManagedBy(m, &TestDefaulterObject{})
			build(webhookBuilder)
			err = webhookBuilder.
				RecoverPanic(true).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestDefaulterObject"
    },
    "resource":{
      "group":"",
      "version":"v1",
      "resource":"testdefaulter"
    },
    "namespace":"default",
    "operation":"CREATE",
    "object":{
      "replica":1,
      "panic":true
    },
    "oldObject":null
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testDefaulterGVK)
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable fields")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":false`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":500`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"message":"panic: fake panic test [recovered]`))
		},
		Entry("CustomDefaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithCustomDefaulter(&TestCustomDefaulter{})
		}),
		Entry("Defaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithDefaulter(&testDefaulter{})
		}),
	)

	DescribeTable("should scaffold a custom validating webhook",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestValidatorObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestValidatorObject{}, &TestValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhook := WebhookManagedBy(m, &TestValidatorObject{})
			build(webhook)
			err = webhook.WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
				return admission.DefaultLogConstructor(testingLogger, req)
			}).
				WithContextFunc(func(ctx context.Context, request *http.Request) context.Context {
					return context.WithValue(ctx, userAgentCtxKey, request.Header.Get(userAgentHeader))
				}).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"foo.test.org",
      "version":"v1",
      "kind":"TestValidatorObject"
    },
    "resource":{
      "group":"foo.test.org",
      "version":"v1",
      "resource":"testvalidator"
    },
    "namespace":"default",
    "name":"foo",
    "operation":"UPDATE",
    "object":{
      "replica":1
    },
    "oldObject":{
      "replica":2
    }
  }
}`)
			readerWithCxt := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"foo.test.org",
      "version":"v1",
      "kind":"TestValidatorObject"
    },
    "resource":{
      "group":"foo.test.org",
      "version":"v1",
      "resource":"testvalidator"
    },
    "namespace":"default",
    "name":"foo",
    "operation":"UPDATE",
    "object":{
      "replica":1
    },
    "oldObject":{
      "replica":1
    }
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path that doesn't exist")
			path := generateMutatePath(testValidatorGVK)
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))

			By("sending a request to a validating webhook path")
			path = generateValidatePath(testValidatorGVK)
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"allowed":false`))
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"code":403`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Validating object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testvalidator"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))

			By("sending a request to a validating webhook with context header validation")
			path = generateValidatePath(testValidatorGVK)
			_, err = readerWithCxt.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req = httptest.NewRequest("POST", svcBaseAddr+path, readerWithCxt)
			req.Header.Add("Content-Type", "application/json")
			req.Header.Add(userAgentHeader, userAgentValue)
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"allowed":true`))
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"code":200`))
		},
		Entry("CustomValidator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithCustomValidator(&TestCustomValidator{})
		}),
		Entry("Validator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithValidator(&testValidator{})
		}),
	)

	DescribeTable("should scaffold a custom validating webhook with a custom path",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestValidatorObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestValidatorObject{}, &TestValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			customPath := "/custom-validating-path"
			webhookBuilder := WebhookManagedBy(m, &TestValidatorObject{})
			build(webhookBuilder)
			err = webhookBuilder.WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
				return admission.DefaultLogConstructor(testingLogger, req)
			}).
				WithValidatorCustomPath(customPath).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"foo.test.org",
      "version":"v1",
      "kind":"TestValidatorObject"
    },
    "resource":{
      "group":"foo.test.org",
      "version":"v1",
      "resource":"testvalidator"
    },
    "namespace":"default",
    "name":"foo",
    "operation":"UPDATE",
    "object":{
      "replica":1
    },
    "oldObject":{
      "replica":2
    }
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a valiting webhook path that have been overriten by a custom path")
			path, err := generateCustomPath(customPath)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"allowed":false`))
			ExpectWithOffset(1, w.Body.String()).To(ContainSubstring(`"code":403`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Validating object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testvalidator"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))

			By("sending a request to a validating webhook path")
			path = generateValidatePath(testValidatorGVK)
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))
		},
		Entry("CustomValidator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithCustomValidator(&TestCustomValidator{})
		}),
		Entry("Validator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithValidator(&testValidator{})
		}),
	)

	DescribeTable("should scaffold a custom validating webhook which recovers from panics",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestValidatorObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestValidatorObject{}, &TestValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhookBuilder := WebhookManagedBy(m, &TestValidatorObject{})
			build(webhookBuilder)
			err = webhookBuilder.RecoverPanic(true).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestValidatorObject"
    },
    "resource":{
      "group":"",
      "version":"v1",
      "resource":"testvalidator"
    },
    "namespace":"default",
    "operation":"CREATE",
    "object":{
      "replica":2,
      "panic":true
    }
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a validating webhook path")
			path := generateValidatePath(testValidatorGVK)
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":false`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":500`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"message":"panic: fake panic test [recovered]`))
		},
		Entry("CustomValidator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithCustomValidator(&TestCustomValidator{})
		}),
		Entry("Validator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithValidator(&testValidator{})
		}),
	)

	DescribeTable("should scaffold a custom validating webhook to validate deletes",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestValidatorObject])) {
			By("creating a controller manager")
			ctx, cancel := context.WithCancel(specCtx)

			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestValidatorObject{}, &TestValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhookBuilder := WebhookManagedBy(m, &TestValidatorObject{})
			build(webhookBuilder)
			err = webhookBuilder.Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestValidatorObject"
    },
    "resource":{
      "group":"",
      "version":"v1",
      "resource":"testvalidator"
    },
    "namespace":"default",
    "operation":"DELETE",
    "object":null,
    "oldObject":{
      "replica":1
    }
  }
}`)

			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a validating webhook path to check for failed delete")
			path := generateValidatePath(testValidatorGVK)
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":false`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":403`))

			reader = strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestValidatorObject"
    },
    "resource":{
      "group":"",
      "version":"v1",
      "resource":"testvalidator"
    },
    "namespace":"default",
    "operation":"DELETE",
    "object":null,
    "oldObject":{
      "replica":0
    }
  }
}`)
			By("sending a request to a validating webhook path with correct request")
			path = generateValidatePath(testValidatorGVK)
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":true`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":200`))
		},
		Entry("CustomValidator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithCustomValidator(&TestCustomValidator{})
		}),
		Entry("Validator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithValidator(&testValidator{})
		}),
	)

	DescribeTable("should scaffold a custom defaulting and validating webhook",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestDefaultValidator])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestDefaultValidator{}, &TestDefaultValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhookBuilder := WebhookManagedBy(m, &TestDefaultValidator{})
			build(webhookBuilder)
			err = webhookBuilder.
				WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
					return admission.DefaultLogConstructor(testingLogger, req)
				}).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"foo.test.org",
      "version":"v1",
      "kind":"TestDefaultValidator"
    },
    "resource":{
      "group":"foo.test.org",
      "version":"v1",
      "resource":"testdefaultvalidator"
    },
    "namespace":"default",
    "name":"foo",
    "operation":"UPDATE",
    "object":{
      "replica":1
    },
    "oldObject":{
      "replica":2
    }
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testDefaultValidatorGVK)
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable fields")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":true`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"patch":`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":200`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Defaulting object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testdefaultvalidator"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))

			By("sending a request to a validating webhook path")
			path = generateValidatePath(testDefaultValidatorGVK)
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":false`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":403`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Validating object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testdefaultvalidator"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))
		},
		Entry("CustomDefaulter + CustomValidator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithCustomDefaulter(&TestCustomDefaultValidator{})
			b.WithCustomValidator(&TestCustomDefaultValidator{})
		}),
		Entry("CustomDefaulter + Validator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithCustomDefaulter(&TestCustomDefaultValidator{})
			b.WithValidator(&testDefaultValidatorValidator{})
		}),
		Entry("Defaulter + CustomValidator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithDefaulter(&testValidatorDefaulter{})
			b.WithCustomValidator(&TestCustomDefaultValidator{})
		}),
		Entry("Defaulter + Validator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithDefaulter(&testValidatorDefaulter{})
			b.WithValidator(&testDefaultValidatorValidator{})
		}),
	)

	DescribeTable("should scaffold a custom defaulting and validating webhook with a custom path for each of them",
		func(specCtx SpecContext, build func(*WebhookBuilder[*TestDefaultValidator])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestDefaultValidator{}, &TestDefaultValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			validatingCustomPath := "/custom-validating-path"
			defaultingCustomPath := "/custom-defaulting-path"
			webhookBuilder := WebhookManagedBy(m, &TestDefaultValidator{})
			build(webhookBuilder)
			err = webhookBuilder.
				WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
					return admission.DefaultLogConstructor(testingLogger, req)
				}).
				WithValidatorCustomPath(validatingCustomPath).
				WithDefaulterCustomPath(defaultingCustomPath).
				Complete()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			ExpectWithOffset(1, svr).NotTo(BeNil())

			reader := strings.NewReader(admissionReviewGV + admissionReviewVersion + `",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"foo.test.org",
      "version":"v1",
      "kind":"TestDefaultValidator"
    },
    "resource":{
      "group":"foo.test.org",
      "version":"v1",
      "resource":"testdefaultvalidator"
    },
    "namespace":"default",
    "name":"foo",
    "operation":"UPDATE",
    "object":{
      "replica":1
    },
    "oldObject":{
      "replica":2
    }
  }
}`)

			ctx, cancel := context.WithCancel(specCtx)
			cancel()
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path that have been overriten by the custom path")
			path, err := generateCustomPath(defaultingCustomPath)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req := httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable fields")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":true`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"patch":`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":200`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Defaulting object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testdefaultvalidator"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))

			By("sending a request to a mutating webhook path")
			path = generateMutatePath(testDefaultValidatorGVK)
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))

			By("sending a request to a valiting webhook path that have been overriten by a custom path")
			path, err = generateCustomPath(validatingCustomPath)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			_, err = reader.Seek(0, 0)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":false`))
			ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":403`))
			EventuallyWithOffset(1, logBuffer).Should(gbytes.Say(`"msg":"Validating object","object":{"name":"foo","namespace":"default"},"namespace":"default","name":"foo","resource":{"group":"foo.test.org","version":"v1","resource":"testdefaultvalidator"},"user":"","requestID":"07e52e8d-4513-11e9-a716-42010a800270"`))

			By("sending a request to a validating webhook path")
			path = generateValidatePath(testValidatorGVK)
			req = httptest.NewRequest("POST", svcBaseAddr+path, reader)
			req.Header.Add("Content-Type", "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux().ServeHTTP(w, req)
			ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))
		},
		Entry("CustomDefaulter + CustomValidator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithCustomDefaulter(&TestCustomDefaultValidator{})
			b.WithCustomValidator(&TestCustomDefaultValidator{})
		}),
		Entry("CustomDefaulter + Validator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithCustomDefaulter(&TestCustomDefaultValidator{})
			b.WithValidator(&testDefaultValidatorValidator{})
		}),
		Entry("Defaulter + CustomValidator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithDefaulter(&testValidatorDefaulter{})
			b.WithCustomValidator(&TestCustomDefaultValidator{})
		}),
		Entry("Defaulter + Validator", func(b *WebhookBuilder[*TestDefaultValidator]) {
			b.WithDefaulter(&testValidatorDefaulter{})
			b.WithValidator(&testDefaultValidatorValidator{})
		}),
	)

	It("should not scaffold a custom defaulting and a custom validating webhook with the same custom path", func() {
		By("creating a controller manager")
		m, err := manager.New(cfg, manager.Options{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		By("registering the type in the Scheme")
		builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
		builder.Register(&TestDefaultValidator{}, &TestDefaultValidatorList{})
		err = builder.AddToScheme(m.GetScheme())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = WebhookManagedBy(m, &TestDefaultValidator{}).
			WithCustomDefaulter(&TestCustomDefaultValidator{}).
			WithCustomValidator(&TestCustomDefaultValidator{}).
			WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
				return admission.DefaultLogConstructor(testingLogger, req)
			}).
			WithCustomPath(customPath).
			Complete()
		ExpectWithOffset(1, err).To(HaveOccurred())
	})

	DescribeTable("should not scaffold a custom defaulting when setting a custom path and a defaulting custom path",
		func(build func(*WebhookBuilder[*TestDefaulterObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
			builder.Register(&TestDefaulterObject{}, &TestDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhookBuilder := WebhookManagedBy(m, &TestDefaulterObject{})
			build(webhookBuilder)
			err = webhookBuilder.
				WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
					return admission.DefaultLogConstructor(testingLogger, req)
				}).
				WithDefaulterCustomPath(customPath).
				WithCustomPath(customPath).
				Complete()
			ExpectWithOffset(1, err).To(HaveOccurred())
		},
		Entry("CustomDefaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithCustomDefaulter(&TestCustomDefaulter{})
		}),
		Entry("Defaulter", func(b *WebhookBuilder[*TestDefaulterObject]) {
			b.WithDefaulter(&testDefaulter{})
		}),
	)

	DescribeTable("should not scaffold a custom validating when setting a custom path and a validating custom path",
		func(build func(*WebhookBuilder[*TestValidatorObject])) {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestValidatorObject{}, &TestValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			webhookBuilder := WebhookManagedBy(m, &TestValidatorObject{})
			build(webhookBuilder)
			err = webhookBuilder.
				WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
					return admission.DefaultLogConstructor(testingLogger, req)
				}).
				WithValidatorCustomPath(customPath).
				WithCustomPath(customPath).
				Complete()
			ExpectWithOffset(1, err).To(HaveOccurred())
		},
		Entry("CustomValidator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithCustomValidator(&TestCustomValidator{})
		}),
		Entry("Validator", func(b *WebhookBuilder[*TestValidatorObject]) {
			b.WithValidator(&testValidator{})
		}),
	)

	It("should error if both a defaulter and a custom defaulter are set", func() {
		m, err := manager.New(cfg, manager.Options{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
		builder.Register(&TestDefaulterObject{}, &TestDefaulterList{})
		err = builder.AddToScheme(m.GetScheme())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = WebhookManagedBy(m, &TestDefaulterObject{}).
			WithDefaulter(&testDefaulter{}).
			WithCustomDefaulter(&TestCustomDefaulter{}).
			Complete()
		ExpectWithOffset(1, err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("only one of Defaulter or CustomDefaulter can be set"))
	})
	It("should error if both a validator and a custom validator are set", func() {
		m, err := manager.New(cfg, manager.Options{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
		builder.Register(&TestValidatorObject{}, &TestValidatorList{})
		err = builder.AddToScheme(m.GetScheme())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = WebhookManagedBy(m, &TestValidatorObject{}).
			WithValidator(&testValidator{}).
			WithCustomValidator(&TestCustomValidator{}).
			Complete()
		ExpectWithOffset(1, err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("only one of Validator or CustomValidator can be set"))
	})
}

// TestDefaulter.
var _ runtime.Object = &TestDefaulterObject{}

const testDefaulterKind = "TestDefaulterObject"

type TestDefaulterObject struct {
	Replica int  `json:"replica,omitempty"`
	Panic   bool `json:"panic,omitempty"`
}

var testDefaulterGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: testDefaulterKind}

func (d *TestDefaulterObject) GetObjectKind() schema.ObjectKind { return d }
func (d *TestDefaulterObject) DeepCopyObject() runtime.Object {
	return &TestDefaulterObject{
		Replica: d.Replica,
	}
}

func (d *TestDefaulterObject) GroupVersionKind() schema.GroupVersionKind {
	return testDefaulterGVK
}

func (d *TestDefaulterObject) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestDefaulterList{}

type TestDefaulterList struct{}

func (*TestDefaulterList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestDefaulterList) DeepCopyObject() runtime.Object   { return nil }

// TestValidator.
var _ runtime.Object = &TestValidatorObject{}

const testValidatorKind = "TestValidatorObject"

type TestValidatorObject struct {
	Replica int  `json:"replica,omitempty"`
	Panic   bool `json:"panic,omitempty"`
}

var testValidatorGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: testValidatorKind}

func (v *TestValidatorObject) GetObjectKind() schema.ObjectKind { return v }
func (v *TestValidatorObject) DeepCopyObject() runtime.Object {
	return &TestValidatorObject{
		Replica: v.Replica,
	}
}

func (v *TestValidatorObject) GroupVersionKind() schema.GroupVersionKind {
	return testValidatorGVK
}

func (v *TestValidatorObject) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestValidatorList{}

type TestValidatorList struct{}

func (*TestValidatorList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestValidatorList) DeepCopyObject() runtime.Object   { return nil }

// TestDefaultValidator.
var _ runtime.Object = &TestDefaultValidator{}

const testDefaultValidatorKind = "TestDefaultValidator"

type TestDefaultValidator struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	Replica int `json:"replica,omitempty"`
}

var testDefaultValidatorGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestDefaultValidator"}

func (dv *TestDefaultValidator) GetObjectKind() schema.ObjectKind { return dv }
func (dv *TestDefaultValidator) DeepCopyObject() runtime.Object {
	return &TestDefaultValidator{
		Replica: dv.Replica,
	}
}

func (dv *TestDefaultValidator) GroupVersionKind() schema.GroupVersionKind {
	return testDefaultValidatorGVK
}

func (dv *TestDefaultValidator) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestDefaultValidatorList{}

type TestDefaultValidatorList struct{}

func (*TestDefaultValidatorList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestDefaultValidatorList) DeepCopyObject() runtime.Object   { return nil }

type TestCustomDefaulter struct{}

func (*TestCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	d := obj.(*TestDefaulterObject) //nolint:ifshort
	return (&testDefaulter{}).Default(ctx, d)
}

type testDefaulter struct{}

func (*testDefaulter) Default(ctx context.Context, obj *TestDefaulterObject) error {
	logf.FromContext(ctx).Info("Defaulting object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testDefaulterKind {
		return fmt.Errorf("expected Kind TestDefaulter got %q", req.Kind.Kind)
	}

	if obj.Panic {
		panic("fake panic test")
	}

	if obj.Replica < 2 {
		obj.Replica = 2
	}

	return nil
}

//nolint:staticcheck
var _ admission.CustomDefaulter = &TestCustomDefaulter{}

type TestCustomValidator struct{}

func (*TestCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	v := obj.(*TestValidatorObject) //nolint:ifshort
	return (&testValidator{}).ValidateCreate(ctx, v)
}

func (*TestCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	v := newObj.(*TestValidatorObject)
	old := oldObj.(*TestValidatorObject)
	return (&testValidator{}).ValidateUpdate(ctx, old, v)
}

func (*TestCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	v := obj.(*TestValidatorObject) //nolint:ifshort
	return (&testValidator{}).ValidateDelete(ctx, v)
}

type testValidator struct{}

func (*testValidator) ValidateCreate(ctx context.Context, obj *TestValidatorObject) (admission.Warnings, error) {
	logf.FromContext(ctx).Info("Validating object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testValidatorKind {
		return nil, fmt.Errorf("expected Kind TestValidator got %q", req.Kind.Kind)
	}

	if obj.Panic {
		panic("fake panic test")
	}
	if obj.Replica < 0 {
		return nil, errors.New("number of replica should be greater than or equal to 0")
	}

	return nil, nil
}

func (*testValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *TestValidatorObject) (admission.Warnings, error) {
	logf.FromContext(ctx).Info("Validating object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testValidatorKind {
		return nil, fmt.Errorf("expected Kind TestValidator got %q", req.Kind.Kind)
	}

	if newObj.Replica < 0 {
		return nil, errors.New("number of replica should be greater than or equal to 0")
	}
	if newObj.Replica < oldObj.Replica {
		return nil, fmt.Errorf("new replica %v should not be fewer than old replica %v", newObj.Replica, oldObj.Replica)
	}

	userAgent, ok := ctx.Value(userAgentCtxKey).(string)
	if ok && userAgent != userAgentValue {
		return nil, fmt.Errorf("expected %s value is %q in TestCustomValidator got %q", userAgentCtxKey, userAgentValue, userAgent)
	}

	return nil, nil
}

func (*testValidator) ValidateDelete(ctx context.Context, obj *TestValidatorObject) (admission.Warnings, error) {
	logf.FromContext(ctx).Info("Validating object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testValidatorKind {
		return nil, fmt.Errorf("expected Kind TestValidator got %q", req.Kind.Kind)
	}

	if obj.Replica > 0 {
		return nil, errors.New("number of replica should be less than or equal to 0 to delete")
	}
	return nil, nil
}

//nolint:staticcheck
var _ admission.CustomValidator = &TestCustomValidator{}

// TestCustomDefaultValidator for default
type TestCustomDefaultValidator struct{}

func (*TestCustomDefaultValidator) Default(ctx context.Context, obj runtime.Object) error {
	logf.FromContext(ctx).Info("Defaulting object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testDefaultValidatorKind {
		return fmt.Errorf("expected Kind TestDefaultValidator got %q", req.Kind.Kind)
	}

	d := obj.(*TestDefaultValidator) //nolint:ifshort

	if d.Replica < 2 {
		d.Replica = 2
	}
	return nil
}

//nolint:staticcheck
var _ admission.CustomDefaulter = &TestCustomDefaulter{}

// TestCustomDefaultValidator for validation

func (*TestCustomDefaultValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logf.FromContext(ctx).Info("Validating object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testDefaultValidatorKind {
		return nil, fmt.Errorf("expected Kind TestDefaultValidator got %q", req.Kind.Kind)
	}

	v := obj.(*TestDefaultValidator) //nolint:ifshort
	if v.Replica < 0 {
		return nil, errors.New("number of replica should be greater than or equal to 0")
	}
	return nil, nil
}

func (*TestCustomDefaultValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	logf.FromContext(ctx).Info("Validating object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testDefaultValidatorKind {
		return nil, fmt.Errorf("expected Kind TestDefaultValidator got %q", req.Kind.Kind)
	}

	v := newObj.(*TestDefaultValidator)
	old := oldObj.(*TestDefaultValidator)
	if v.Replica < 0 {
		return nil, errors.New("number of replica should be greater than or equal to 0")
	}
	if v.Replica < old.Replica {
		return nil, fmt.Errorf("new replica %v should not be fewer than old replica %v", v.Replica, old.Replica)
	}
	return nil, nil
}

func (*TestCustomDefaultValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logf.FromContext(ctx).Info("Validating object")
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("expected admission.Request in ctx: %w", err)
	}
	if req.Kind.Kind != testDefaultValidatorKind {
		return nil, fmt.Errorf("expected Kind TestDefaultValidator got %q", req.Kind.Kind)
	}

	v := obj.(*TestDefaultValidator) //nolint:ifshort
	if v.Replica > 0 {
		return nil, errors.New("number of replica should be less than or equal to 0 to delete")
	}
	return nil, nil
}

//nolint:staticcheck
var _ admission.CustomValidator = &TestCustomValidator{}

type testValidatorDefaulter struct{}

func (*testValidatorDefaulter) Default(ctx context.Context, obj *TestDefaultValidator) error {
	return (&TestCustomDefaultValidator{}).Default(ctx, obj)
}

type testDefaultValidatorValidator struct{}

func (*testDefaultValidatorValidator) ValidateCreate(ctx context.Context, obj *TestDefaultValidator) (admission.Warnings, error) {
	return (&TestCustomDefaultValidator{}).ValidateCreate(ctx, obj)
}

func (*testDefaultValidatorValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *TestDefaultValidator) (admission.Warnings, error) {
	return (&TestCustomDefaultValidator{}).ValidateUpdate(ctx, oldObj, newObj)
}

func (*testDefaultValidatorValidator) ValidateDelete(ctx context.Context, obj *TestDefaultValidator) (admission.Warnings, error) {
	return (&TestCustomDefaultValidator{}).ValidateDelete(ctx, obj)
}
