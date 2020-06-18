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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var _ = Describe("application", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
		newController = controller.New
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("New with registration", func() {
		It("should scaffold a defaulting webhook if the registered type implements the Defaulter interface", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testRegisteredDefaulterGVK.GroupVersion()}
			builder.Register(&TestRegisteredDefaulter{}, &TestRegisteredDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestRegisteredDefaulter{}).
				WithDefaulter(&testDefaulterService{}).
				Complete()

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
      "kind":"TestRegisteredDefaulter"
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

			stopCh := make(chan struct{})
			close(stopCh)
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(stopCh)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testRegisteredDefaulterGVK)
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
			path = generateValidatePath(testRegisteredDefaulterGVK)
			req = httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w = httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("should be able to use dependencies given to the defaulting service while defaulting", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testRegisteredDefaulterGVK.GroupVersion()}
			builder.Register(&TestRegisteredDefaulter{}, &TestRegisteredDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			someDependency := &SomeDependency{}
			err = WebhookManagedBy(m).
				For(&TestRegisteredDefaulter{}).
				WithDefaulter(&testDefaulterService{someDependency}).
				Complete()

			Expect(err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			Expect(svr).NotTo(BeNil())

			reader := strings.NewReader("")

			stopCh := make(chan struct{})
			close(stopCh)

			err = svr.Start(stopCh)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testRegisteredDefaulterGVK)
			req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)

			Expect(someDependency.DoSomethingWasCalled()).To(BeTrue())
		})

		It("should be able to use dependencies given to the validating service while validating", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testRegisteredValidatorGVK.GroupVersion()}
			builder.Register(&TestRegisteredValidator{}, &TestRegisteredValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			someDependency := &SomeDependency{}
			err = WebhookManagedBy(m).
				For(&TestRegisteredValidator{}).
				WithValidator(&testValidatorService{someDependency}).
				Complete()
			Expect(err).NotTo(HaveOccurred())
			svr := m.GetWebhookServer()
			Expect(svr).NotTo(BeNil())

			stopCh := make(chan struct{})
			close(stopCh)

			err = svr.Start(stopCh)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a validating webhook path")
			path := generateValidatePath(testRegisteredValidatorGVK)
			reader := strings.NewReader(`{
  "kind":"AdmissionReview",
  "apiVersion":"admission.k8s.io/v1beta1",
  "request":{
    "uid":"07e52e8d-4513-11e9-a716-42010a800270",
    "kind":{
      "group":"",
      "version":"v1",
      "kind":"TestRegisteredValidator"
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
			req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))

			Expect(someDependency.DoSomethingWasCalled()).To(BeTrue())
		})

		It("should scaffold a validating webhook if the type implements the Validator interface", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testRegisteredValidatorGVK.GroupVersion()}
			builder.Register(&TestRegisteredValidator{}, &TestRegisteredValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestRegisteredValidator{}).
				WithValidator(&testValidatorService{}).
				Complete()
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
      "kind":"TestRegisteredValidator"
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

			stopCh := make(chan struct{})
			close(stopCh)
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(stopCh)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path that doesn't exist")
			path := generateMutatePath(testRegisteredValidatorGVK)
			req := httptest.NewRequest("POST", "http://svc-name.svc-ns.svc"+path, reader)
			req.Header.Add(http.CanonicalHeaderKey("Content-Type"), "application/json")
			w := httptest.NewRecorder()
			svr.WebhookMux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusNotFound))

			By("sending a request to a validating webhook path")
			path = generateValidatePath(testRegisteredValidatorGVK)
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
			builder.Register(&TestRegisteredDefaultValidator{}, &TestRegisteredDefaultValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestRegisteredDefaultValidator{}).
				WithDefaulter(&testRegisteredDefaulterService{}).
				WithValidator(&testRegisteredValidatorService{}).
				Complete()
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
      "kind":"TestRegisteredDefaultValidator"
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

			stopCh := make(chan struct{})
			close(stopCh)
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(stopCh)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a mutating webhook path")
			path := generateMutatePath(testRegisteredDefaultValidatorGVK)
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
			path = generateValidatePath(testRegisteredDefaultValidatorGVK)
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
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testRegisteredValidatorGVK.GroupVersion()}
			builder.Register(&TestRegisteredValidator{}, &TestRegisteredValidatorList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			err = WebhookManagedBy(m).
				For(&TestRegisteredValidator{}).
				WithValidator(&testValidatorService{}).
				Complete()

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
      "kind":"TestRegisteredValidator"
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
			stopCh := make(chan struct{})
			close(stopCh)
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(stopCh)
			if err != nil && !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("sending a request to a validating webhook path to check for failed delete")
			path := generateValidatePath(testRegisteredValidatorGVK)
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
      "kind":"TestRegisteredValidator"
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
			path = generateValidatePath(testRegisteredValidatorGVK)
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

// TestRegisteredDefaulter
var _ runtime.Object = &TestRegisteredDefaulter{}

type TestRegisteredDefaulter struct {
	Replica int `json:"replica,omitempty"`
}

var testRegisteredDefaulterGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestRegisteredDefaulter"}

func (d *TestRegisteredDefaulter) GetObjectKind() schema.ObjectKind { return d }
func (d *TestRegisteredDefaulter) DeepCopyObject() runtime.Object {
	return &TestRegisteredDefaulter{
		Replica: d.Replica,
	}
}

func (d *TestRegisteredDefaulter) GroupVersionKind() schema.GroupVersionKind {
	return testRegisteredDefaulterGVK
}

func (d *TestRegisteredDefaulter) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestRegisteredDefaulterList{}

type TestRegisteredDefaulterList struct{}

func (*TestRegisteredDefaulterList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestRegisteredDefaulterList) DeepCopyObject() runtime.Object   { return nil }

// TestRegisteredValidator
var _ runtime.Object = &TestRegisteredValidator{}

type TestRegisteredValidator struct {
	Replica int `json:"replica,omitempty"`
}

var testRegisteredValidatorGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestRegisteredValidator"}

func (v *TestRegisteredValidator) GetObjectKind() schema.ObjectKind { return v }
func (v *TestRegisteredValidator) DeepCopyObject() runtime.Object {
	return &TestRegisteredValidator{
		Replica: v.Replica,
	}
}

func (v *TestRegisteredValidator) GroupVersionKind() schema.GroupVersionKind {
	return testRegisteredValidatorGVK
}

func (v *TestRegisteredValidator) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestRegisteredValidatorList{}

type TestRegisteredValidatorList struct{}

func (*TestRegisteredValidatorList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestRegisteredValidatorList) DeepCopyObject() runtime.Object   { return nil }

// TestRegisteredDefaultValidator
var _ runtime.Object = &TestRegisteredDefaultValidator{}

type TestRegisteredDefaultValidator struct {
	Replica int `json:"replica,omitempty"`
}

var testRegisteredDefaultValidatorGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestRegisteredDefaultValidator"}

func (dv *TestRegisteredDefaultValidator) GetObjectKind() schema.ObjectKind { return dv }
func (dv *TestRegisteredDefaultValidator) DeepCopyObject() runtime.Object {
	return &TestRegisteredDefaultValidator{
		Replica: dv.Replica,
	}
}

func (dv *TestRegisteredDefaultValidator) GroupVersionKind() schema.GroupVersionKind {
	return testDefaultValidatorGVK
}

func (dv *TestRegisteredDefaultValidator) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

var _ runtime.Object = &TestRegisteredDefaultValidatorList{}

type TestRegisteredDefaultValidatorList struct{}

func (*TestRegisteredDefaultValidatorList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestRegisteredDefaultValidatorList) DeepCopyObject() runtime.Object   { return nil }

type testDefaulterService struct {
	dep *SomeDependency
}

func (t *testDefaulterService) Default(object runtime.Object) {
	if t.dep != nil {
		t.dep.doSomething()
	}

	registeredDefaulter, ok := object.(*TestRegisteredDefaulter)
	if !ok {
		return
	}

	if registeredDefaulter.Replica < 2 {
		registeredDefaulter.Replica = 2
	}
}

type testValidatorService struct {
	dep *SomeDependency
}

func (t *testValidatorService) ValidateCreate(new runtime.Object) error {
	v, ok := new.(*TestRegisteredValidator)
	if !ok {
		return errors.New("not a validator")
	}

	if v.Replica < 0 {
		return errors.New("number of replica should be greater than or equal to 0")
	}
	return nil
}

func (t *testValidatorService) ValidateUpdate(new runtime.Object, old runtime.Object) error {
	if t.dep != nil {
		t.dep.doSomething()
	}

	v, ok := new.(*TestRegisteredValidator)

	if !ok {
		return errors.New("not a validator")
	}
	if v.Replica < 0 {
		return errors.New("number of replica should be greater than or equal to 0")
	}
	if oldObj, ok := old.(*TestRegisteredValidator); !ok {
		return fmt.Errorf("the old object is expected to be %T", oldObj)
	} else if v.Replica < oldObj.Replica {
		return fmt.Errorf("new replica %v should not be fewer than old replica %v", v.Replica, oldObj.Replica)
	}
	return nil
}

func (t *testValidatorService) ValidateDelete(existing runtime.Object) error {
	v, ok := existing.(*TestRegisteredValidator)
	if !ok {
		return errors.New("not a validator")
	}

	if v.Replica > 0 {
		return errors.New("number of replica should be less than or equal to 0 to delete")
	}

	return nil
}

type testRegisteredDefaulterService struct {
}

type testRegisteredValidatorService struct {
}

func (t *testRegisteredDefaulterService) Default(object runtime.Object) {
	dv, ok := object.(*TestRegisteredDefaultValidator)
	if !ok {
		return
	}

	if dv.Replica < 2 {
		dv.Replica = 2
	}
}

func (t *testRegisteredValidatorService) ValidateCreate(new runtime.Object) error {
	panic("implement me")
}

func (t *testRegisteredValidatorService) ValidateUpdate(new runtime.Object, old runtime.Object) error {
	panic("implement me")
}

func (t *testRegisteredValidatorService) ValidateDelete(existing runtime.Object) error {
	panic("implement me")
}

type SomeDependency struct {
	wasCalled bool
}

func (d *SomeDependency) doSomething() {
	d.wasCalled = true
}

func (d *SomeDependency) DoSomethingWasCalled() bool {
	return d.wasCalled
}
