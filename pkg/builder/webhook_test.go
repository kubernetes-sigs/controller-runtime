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
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

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
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
		newController = controller.New
	})

	AfterEach(func() {
		close(stop)
	})

	It("should scaffold a defaulting webhook if the type implements the Defaulter interface", func() {
		By("creating a controller manager")
		m, err := manager.New(cfg, manager.Options{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		By("registering the type in the Scheme")
		builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
		builder.Register(&TestDefaulter{}, &TestDefaulterList{})
		err = builder.AddToScheme(m.GetScheme())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = WebhookManagedBy(m).
			For(&TestDefaulter{}).
			Complete()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		svr := m.GetWebhookServer()
		ExpectWithOffset(1, svr).NotTo(BeNil())

		reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/` + admissionReviewVersion + `",
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
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}

		By("sending a request to a mutating webhook path")
		path := generateMutatePath(testDefaulterGVK)
		req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
		By("sanity checking the response contains reasonable fields")
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":true`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"patch":`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":200`))

		By("sending a request to a validating webhook path that doesn't exist")
		path = generateValidatePath(testDefaulterGVK)
		_, err = reader.Seek(0, 0)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
		req.Header.Add("Content-Type", "application/json")
		w = httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))
	})

	It("should scaffold a validating webhook if the type implements the Validator interface", func() {
		By("creating a controller manager")
		m, err := manager.New(cfg, manager.Options{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		By("registering the type in the Scheme")
		builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
		builder.Register(&TestValidator{}, &TestValidatorList{})
		err = builder.AddToScheme(m.GetScheme())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = WebhookManagedBy(m).
			For(&TestValidator{}).
			Complete()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		svr := m.GetWebhookServer()
		ExpectWithOffset(1, svr).NotTo(BeNil())

		reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/` + admissionReviewVersion + `",
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
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}

		By("sending a request to a mutating webhook path that doesn't exist")
		path := generateMutatePath(testValidatorGVK)
		req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusNotFound))

		By("sending a request to a validating webhook path")
		path = generateValidatePath(testValidatorGVK)
		_, err = reader.Seek(0, 0)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
		req.Header.Add("Content-Type", "application/json")
		w = httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
		By("sanity checking the response contains reasonable field")
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":false`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":403`))
	})

	It("should scaffold defaulting and validating webhooks if the type implements both Defaulter and Validator interfaces", func() {
		By("creating a controller manager")
		m, err := manager.New(cfg, manager.Options{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		By("registering the type in the Scheme")
		builder := scheme.Builder{GroupVersion: testDefaultValidatorGVK.GroupVersion()}
		builder.Register(&TestDefaultValidator{}, &TestDefaultValidatorList{})
		err = builder.AddToScheme(m.GetScheme())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = WebhookManagedBy(m).
			For(&TestDefaultValidator{}).
			Complete()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		svr := m.GetWebhookServer()
		ExpectWithOffset(1, svr).NotTo(BeNil())

		reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/` + admissionReviewVersion + `",
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
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}

		By("sending a request to a mutating webhook path")
		path := generateMutatePath(testDefaultValidatorGVK)
		req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
		By("sanity checking the response contains reasonable field")
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":true`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"patch":`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":200`))

		By("sending a request to a validating webhook path")
		path = generateValidatePath(testDefaultValidatorGVK)
		_, err = reader.Seek(0, 0)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
		req.Header.Add("Content-Type", "application/json")
		w = httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
		By("sanity checking the response contains reasonable field")
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":true`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":200`))
	})

	It("should scaffold a validating webhook if the type implements the Validator interface to validate deletes", func() {
		By("creating a controller manager")
		ctx, cancel := context.WithCancel(context.Background())

		m, err := manager.New(cfg, manager.Options{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		By("registering the type in the Scheme")
		builder := scheme.Builder{GroupVersion: testValidatorGVK.GroupVersion()}
		builder.Register(&TestValidator{}, &TestValidatorList{})
		err = builder.AddToScheme(m.GetScheme())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		err = WebhookManagedBy(m).
			For(&TestValidator{}).
			Complete()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		svr := m.GetWebhookServer()
		ExpectWithOffset(1, svr).NotTo(BeNil())

		reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/` + admissionReviewVersion + `",
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
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}

		By("sending a request to a validating webhook path to check for failed delete")
		path := generateValidatePath(testValidatorGVK)
		req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
		By("sanity checking the response contains reasonable field")
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":false`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":403`))

		reader = strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/` + admissionReviewVersion + `",
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
		req.Header.Add("Content-Type", "application/json")
		w = httptest.NewRecorder()
		svr.WebhookMux.ServeHTTP(w, req)
		ExpectWithOffset(1, w.Code).To(Equal(http.StatusOK))
		By("sanity checking the response contains reasonable field")
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"allowed":true`))
		ExpectWithOffset(1, w.Body).To(ContainSubstring(`"code":200`))
	})
}

// TestDefaulter.
var _ runtime.Object = &TestDefaulter{}

type TestDefaulter struct {
	Replica int `json:"replica,omitempty"`
}

var testDefaulterGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestDefaulter"}

func (d *TestDefaulter) GetObjectKind() schema.ObjectKind { return d }
func (d *TestDefaulter) DeepCopyObject() runtime.Object {
	return &TestDefaulter{
		Replica: d.Replica,
	}
}

func (d *TestDefaulter) GroupVersionKind() schema.GroupVersionKind {
	return testDefaulterGVK
}

func (d *TestDefaulter) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestDefaulterList{}

type TestDefaulterList struct{}

func (*TestDefaulterList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestDefaulterList) DeepCopyObject() runtime.Object   { return nil }

func (d *TestDefaulter) Default() {
	if d.Replica < 2 {
		d.Replica = 2
	}
}

// TestValidator.
var _ runtime.Object = &TestValidator{}

type TestValidator struct {
	Replica int `json:"replica,omitempty"`
}

var testValidatorGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestValidator"}

func (v *TestValidator) GetObjectKind() schema.ObjectKind { return v }
func (v *TestValidator) DeepCopyObject() runtime.Object {
	return &TestValidator{
		Replica: v.Replica,
	}
}

func (v *TestValidator) GroupVersionKind() schema.GroupVersionKind {
	return testValidatorGVK
}

func (v *TestValidator) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestValidatorList{}

type TestValidatorList struct{}

func (*TestValidatorList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestValidatorList) DeepCopyObject() runtime.Object   { return nil }

var _ admission.Validator = &TestValidator{}

func (v *TestValidator) ValidateCreate() error {
	if v.Replica < 0 {
		return errors.New("number of replica should be greater than or equal to 0")
	}
	return nil
}

func (v *TestValidator) ValidateUpdate(old runtime.Object) error {
	if v.Replica < 0 {
		return errors.New("number of replica should be greater than or equal to 0")
	}
	if oldObj, ok := old.(*TestValidator); !ok {
		return fmt.Errorf("the old object is expected to be %T", oldObj)
	} else if v.Replica < oldObj.Replica {
		return fmt.Errorf("new replica %v should not be fewer than old replica %v", v.Replica, oldObj.Replica)
	}
	return nil
}

func (v *TestValidator) ValidateDelete() error {
	if v.Replica > 0 {
		return errors.New("number of replica should be less than or equal to 0 to delete")
	}
	return nil
}

// TestDefaultValidator.
var _ runtime.Object = &TestDefaultValidator{}

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

func (dv *TestDefaultValidator) Default() {
	if dv.Replica < 2 {
		dv.Replica = 2
	}
}

var _ admission.Validator = &TestDefaultValidator{}

func (dv *TestDefaultValidator) ValidateCreate() error {
	if dv.Replica < 0 {
		return errors.New("number of replica should be greater than or equal to 0")
	}
	return nil
}

func (dv *TestDefaultValidator) ValidateUpdate(old runtime.Object) error {
	if dv.Replica < 0 {
		return errors.New("number of replica should be greater than or equal to 0")
	}
	return nil
}

func (dv *TestDefaultValidator) ValidateDelete() error {
	if dv.Replica > 0 {
		return errors.New("number of replica should be less than or equal to 0 to delete")
	}
	return nil
}
