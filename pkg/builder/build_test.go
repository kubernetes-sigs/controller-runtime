/*
Copyright 2018 The Kubernetes Authors.

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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("application", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
		getConfig = func() (*rest.Config, error) { return cfg, nil }
		newController = controller.New
		newManager = manager.New
	})

	AfterEach(func() {
		close(stop)
	})

	noop := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) { return reconcile.Result{}, nil })

	Describe("New", func() {
		It("should return success if given valid objects", func() {
			instance, err := SimpleController().
				For(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance).NotTo(BeNil())
		})

		It("should return an error if the Config is invalid", func() {
			getConfig = func() (*rest.Config, error) { return cfg, fmt.Errorf("expected error") }
			instance, err := SimpleController().
				For(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))
			Expect(instance).To(BeNil())
		})

		It("should return an error if there is no GVK for an object", func() {
			instance, err := SimpleController().
				For(&fakeType{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type builder.fakeType"))
			Expect(instance).To(BeNil())

			instance, err = SimpleController().
				For(&appsv1.ReplicaSet{}).
				Owns(&fakeType{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no kind is registered for the type builder.fakeType"))
			Expect(instance).To(BeNil())
		})

		It("should return an error if it cannot create the manager", func() {
			newManager = func(config *rest.Config, options manager.Options) (manager.Manager, error) {
				return nil, fmt.Errorf("expected error")
			}
			instance, err := SimpleController().
				For(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))
			Expect(instance).To(BeNil())
		})

		It("should return an error if it cannot create the controller", func() {
			newController = func(name string, mgr manager.Manager, options controller.Options) (
				controller.Controller, error) {
				return nil, fmt.Errorf("expected error")
			}
			instance, err := SimpleController().
				For(&appsv1.ReplicaSet{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))
			Expect(instance).To(BeNil())
		})

		It("should scaffold a defaulting webhook if the type implements the Defaulter interface", func() {
			By("creating a controller manager")
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("registering the type in the Scheme")
			builder := scheme.Builder{GroupVersion: testDefaulterGVK.GroupVersion()}
			builder.Register(&TestDefaulter{}, &TestDefaulterList{})
			err = builder.AddToScheme(m.GetScheme())
			Expect(err).NotTo(HaveOccurred())

			instance, err := ControllerManagedBy(m).
				For(&TestDefaulter{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance).NotTo(BeNil())
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

			stopCh := make(chan struct{})
			close(stopCh)
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(stopCh)
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

			instance, err := ControllerManagedBy(m).
				For(&TestValidator{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance).NotTo(BeNil())
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

			stopCh := make(chan struct{})
			close(stopCh)
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(stopCh)
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

			instance, err := ControllerManagedBy(m).
				For(&TestDefaultValidator{}).
				Owns(&appsv1.ReplicaSet{}).
				Build(noop)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance).NotTo(BeNil())
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

			stopCh := make(chan struct{})
			close(stopCh)
			// TODO: we may want to improve it to make it be able to inject dependencies,
			// but not always try to load certs and return not found error.
			err = svr.Start(stopCh)
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
	})

	Describe("Start with SimpleController", func() {
		It("should Reconcile Owns objects", func(done Done) {
			bldr := SimpleController().
				ForType(&appsv1.Deployment{}).
				WithConfig(cfg).
				Owns(&appsv1.ReplicaSet{})
			doReconcileTest("1", stop, bldr, nil, false)

			close(done)
		}, 10)

		It("should Reconcile Owns objects with a Manager", func(done Done) {
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			bldr := SimpleController().
				WithManager(m).
				For(&appsv1.Deployment{}).
				Owns(&appsv1.ReplicaSet{})
			doReconcileTest("2", stop, bldr, m, false)
			close(done)
		}, 10)
	})

	Describe("Start with ControllerManagedBy", func() {
		It("should Reconcile Owns objects", func(done Done) {
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			bldr := ControllerManagedBy(m).
				For(&appsv1.Deployment{}).
				Owns(&appsv1.ReplicaSet{})
			doReconcileTest("3", stop, bldr, m, false)
			close(done)
		}, 10)

		It("should Reconcile Watches objects", func(done Done) {
			m, err := manager.New(cfg, manager.Options{})
			Expect(err).NotTo(HaveOccurred())

			bldr := ControllerManagedBy(m).
				For(&appsv1.Deployment{}).
				Watches( // Equivalent of Owns
					&source.Kind{Type: &appsv1.ReplicaSet{}},
					&handler.EnqueueRequestForOwner{OwnerType: &appsv1.Deployment{}, IsController: true})
			doReconcileTest("4", stop, bldr, m, true)
			close(done)
		}, 10)
	})
})

func doReconcileTest(nameSuffix string, stop chan struct{}, blder *Builder, mgr manager.Manager, complete bool) {
	deployName := "deploy-name-" + nameSuffix
	rsName := "rs-name-" + nameSuffix

	By("Creating the application")
	ch := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		defer GinkgoRecover()
		if !strings.HasSuffix(req.Name, nameSuffix) {
			// From different test, ignore this request.  Etcd is shared across tests.
			return reconcile.Result{}, nil
		}
		ch <- req
		return reconcile.Result{}, nil
	})

	instance := mgr
	if complete {
		err := blder.Complete(fn)
		Expect(err).NotTo(HaveOccurred())
	} else {
		var err error
		instance, err = blder.Build(fn)
		Expect(err).NotTo(HaveOccurred())
	}

	// Manager should match
	if mgr != nil {
		Expect(instance).To(Equal(mgr))
	}

	By("Starting the application")
	go func() {
		defer GinkgoRecover()
		Expect(instance.Start(stop)).NotTo(HaveOccurred())
		By("Stopping the application")
	}()

	By("Creating a Deployment")
	// Expect a Reconcile when the Deployment is managedObjects.
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      deployName,
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
	err := instance.GetClient().Create(context.TODO(), dep)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for the Deployment Reconcile")
	Expect(<-ch).To(Equal(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: deployName}}))

	By("Creating a ReplicaSet")
	// Expect a Reconcile when an Owned object is managedObjects.
	t := true
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      rsName,
			Labels:    dep.Spec.Selector.MatchLabels,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       deployName,
					Kind:       "Deployment",
					APIVersion: "apps/v1",
					Controller: &t,
					UID:        dep.UID,
				},
			},
		},
		Spec: appsv1.ReplicaSetSpec{
			Selector: dep.Spec.Selector,
			Template: dep.Spec.Template,
		},
	}
	err = instance.GetClient().Create(context.TODO(), rs)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for the ReplicaSet Reconcile")
	Expect(<-ch).To(Equal(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: deployName}}))

}

var _ runtime.Object = &fakeType{}

type fakeType struct{}

func (*fakeType) GetObjectKind() schema.ObjectKind { return nil }
func (*fakeType) DeepCopyObject() runtime.Object   { return nil }

// TestDefaulter
var _ runtime.Object = &TestDefaulter{}

type TestDefaulter struct {
	Replica int `json:"replica,omitempty"`
}

var testDefaulterGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestDefaulter"}

func (*TestDefaulter) GetObjectKind() schema.ObjectKind { return nil }
func (d *TestDefaulter) DeepCopyObject() runtime.Object {
	return &TestDefaulter{
		Replica: d.Replica,
	}
}

var _ runtime.Object = &TestDefaulterList{}

type TestDefaulterList struct{}

func (*TestDefaulterList) GetObjectKind() schema.ObjectKind { return nil }
func (*TestDefaulterList) DeepCopyObject() runtime.Object   { return nil }

func (d *TestDefaulter) Default() {
	if d.Replica < 2 {
		d.Replica = 2
	}
}

// TestValidator
var _ runtime.Object = &TestValidator{}

type TestValidator struct {
	Replica int `json:"replica,omitempty"`
}

var testValidatorGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestValidator"}

func (*TestValidator) GetObjectKind() schema.ObjectKind { return nil }
func (v *TestValidator) DeepCopyObject() runtime.Object {
	return &TestValidator{
		Replica: v.Replica,
	}
}

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

// TestDefaultValidator
var _ runtime.Object = &TestDefaultValidator{}

type TestDefaultValidator struct {
	Replica int `json:"replica,omitempty"`
}

var testDefaultValidatorGVK = schema.GroupVersionKind{Group: "foo.test.org", Version: "v1", Kind: "TestDefaultValidator"}

func (*TestDefaultValidator) GetObjectKind() schema.ObjectKind { return nil }
func (dv *TestDefaultValidator) DeepCopyObject() runtime.Object {
	return &TestDefaultValidator{
		Replica: dv.Replica,
	}
}

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
