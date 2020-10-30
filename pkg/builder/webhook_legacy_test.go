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
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var _ = Describe("webhook", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
		newController = controller.New
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("New", func() {
		It("should scaffold a defaulting webhook if the type implements the Defaulter interface", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
			builder.Register(&TestDefaulter{}, &TestDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestDefaulter{}).
				CompleteLegacy()
			Expect(err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			Expect(svr).NotTo(BeNil())

			reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1beta1",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestDefaulter"
    },
    "resource":{
      "group":"",
      "version":"v1",
      "resource":"testdefaulter"
    },
    "namespace":"default",
    "operation":"CREATE",
    "object":{
      "replica":1
    },
    "oldObject":null
  }
}`)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testDefaulterGVK)
			req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable fields")
			Expect(w.Body).To(ContainSubstring(`"allowed":true`))
			Expect(w.Body).To(ContainSubstring(`"patch":`))
			Expect(w.Body).To(ContainSubstring(`"code":200`))

			By("sending a request to a validating webhook path that doesn't exist")
			path = generateValidatePath(testDefaulterGVK)
			req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("should scaffold a validating webhook if the type implements the Validator interface", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestValidator{}, &TestValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestValidator{}).
				CompleteLegacy()
			Expect(err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			Expect(svr).NotTo(BeNil())

			reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1beta1",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestValidator"
    },
    "resource":{
      "group":"",
      "version":"v1",
      "resource":"testvalidator"
    },
    "namespace":"default",
    "operation":"UPDATE",
    "object":{
      "replica":1
    },
    "oldObject":{
      "replica":2
    }
  }
}`)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path that doesn't exist")
			path := generateMutatePath(testValidatorGVK)
			req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusNotFound))

			By("sending a request to a validating webhook path")
			path = generateValidatePath(testValidatorGVK)
			req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			Expect(w.Body).To(ContainSubstring(`"allowed":false`))
			Expect(w.Body).To(ContainSubstring(`"code":403`))
		})

		It("should scaffold defaulting and validating webhooks if the type implements both Defaulter and Validator interfaces", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testDefaultValidatorGVK.GroupVersion()}
			builder.Register(&TestDefaultValidator{}, &TestDefaultValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestDefaultValidator{}).
				CompleteLegacy()
			Expect(err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			Expect(svr).NotTo(BeNil())

			reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1beta1",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestDefaultValidator"
    },
    "resource":{
      "group":"",
      "version":"v1",
      "resource":"testdefaultvalidator"
    },
    "namespace":"default",
    "operation":"CREATE",
    "object":{
      "replica":1
    },
    "oldObject":null
  }
}`)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testDefaultValidatorGVK)
			req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			Expect(w.Body).To(ContainSubstring(`"allowed":true`))
			Expect(w.Body).To(ContainSubstring(`"patch":`))
			Expect(w.Body).To(ContainSubstring(`"code":200`))

			By("sending a request to a validating webhook path")
			path = generateValidatePath(testDefaultValidatorGVK)
			req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			Expect(w.Body).To(ContainSubstring(`"allowed":true`))
			Expect(w.Body).To(ContainSubstring(`"code":200`))
		})

		It("should scaffold a validating webhook if the type implements the Validator interface to validate deletes", func() {
			By("creating a controller manager")
			ctx, cancel := context.WithCancel(context.Background())

			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
			builder.Register(&TestValidator{}, &TestValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestValidator{}).
				CompleteLegacy()
			Expect(err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			Expect(svr).NotTo(BeNil())

			reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1beta1",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestValidator"
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
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(ctx)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a validating webhook path to check for failed delete")
			path := generateValidatePath(testValidatorGVK)
			req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			Expect(w.Body).To(ContainSubstring(`"allowed":false`))
			Expect(w.Body).To(ContainSubstring(`"code":403`))

			reader = strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1beta1",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestValidator"
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
			req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
			By("sanity checking the response contains reasonable field")
			Expect(w.Body).To(ContainSubstring(`"allowed":true`))
			Expect(w.Body).To(ContainSubstring(`"code":200`))

		})
	})
})
