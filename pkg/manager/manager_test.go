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

package manager

import (
	"fmt"

	"github.com/kubernetes-sigs/controller-runtime/pkg/cache"
	"github.com/kubernetes-sigs/controller-runtime/pkg/cache/informertest"
	"github.com/kubernetes-sigs/controller-runtime/pkg/client"
	"github.com/kubernetes-sigs/controller-runtime/pkg/reconcile"
	"github.com/kubernetes-sigs/controller-runtime/pkg/runtime/inject"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

var _ = Describe("controller", func() {
	var stop chan struct{}

	BeforeEach(func() {
		stop = make(chan struct{})
	})

	AfterEach(func() {
		close(stop)
	})

	Describe("New", func() {

		It("should return an error if there is no Config", func() {
			m, err := New(nil, Options{})
			Expect(m).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("must specify Config"))

		})

		It("should return an error if it can't create a RestMapper", func() {
			expected := fmt.Errorf("expected error: RestMapper")
			m, err := New(cfg, Options{
				MapperProvider: func(c *rest.Config) (meta.RESTMapper, error) { return nil, expected },
			})
			Expect(m).To(BeNil())
			Expect(err).To(Equal(expected))

		})

		It("should return an error it can't create a client.Client", func(done Done) {
			m, err := New(cfg, Options{
				newClient: func(config *rest.Config, options client.Options) (client.Client, error) {
					return nil, fmt.Errorf("expected error")
				}})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})

		It("should return an error it can't create a cache.Cache", func(done Done) {
			m, err := New(cfg, Options{
				newCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
					return nil, fmt.Errorf("expected error")
				}})
			Expect(m).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected error"))

			close(done)
		})
	})

	Describe("Staring a Manager", func() {

		It("should Start each Controller", func() {
			// TODO(community): write this
		})

		It("should return an error if it can't start the cache", func(done Done) {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mrg, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())
			mrg.startCache = func(stop <-chan struct{}) error {
				return fmt.Errorf("expected error")
			}
			Expect(m.Start(stop).Error()).To(ContainSubstring("expected error"))

			close(done)
		})

		It("should return an error if any Components fail to Start", func(done Done) {
			//m, err := New(cfg, Options{})
			//Expect(err).NotTo(HaveOccurred())
			//c, err := controller.New("foo", m, controller.Options{})
			//Expect(err).NotTo(HaveOccurred())
			//ctrl, ok := c.(*internalcontroller.Controller)
			//Expect(ok).To(BeTrue())
			//
			//// Make Controller startup fail
			//ctrl.WaitForCache = func(stopCh <-chan struct{}, cacheSyncs ...toolscache.InformerSynced) bool { return false }
			//err = m.Start(stop)
			//Expect(err).To(HaveOccurred())
			//Expect(err.Error()).To(ContainSubstring("caches to sync"))

			close(done)
		})
	})

	Describe("Manager", func() {
		It("should provide a function to get the Config", func() {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mrg, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())
			Expect(m.GetConfig()).To(Equal(mrg.config))
		})

		It("should provide a function to get the Client", func() {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mrg, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())
			Expect(m.GetClient()).To(Equal(mrg.client))
		})

		It("should provide a function to get the Scheme", func() {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mrg, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())
			Expect(m.GetScheme()).To(Equal(mrg.scheme))
		})

		It("should provide a function to get the FieldIndexer", func() {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mrg, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())
			Expect(m.GetFieldIndexer()).To(Equal(mrg.fieldIndexes))
		})
	})

	Describe("Adding a Component", func() {
		It("should immediately start the Component if the ControllerManager has already Started", func() {
			//m, err := manager.New(cfg, manager.Options{})
			//Expect(err).NotTo(HaveOccurred())
			//mrg, ok := m.(*controllerManager)
			//Expect(ok).To(BeTrue())
			//
			//// Make Controller startup fail
			//go func() {
			//	defer GinkgoRecover()
			//	Expect(m.Start(stop)).NotTo(HaveOccurred())
			//}()
			//Eventually(func() bool { return mrg.started }).Should(BeTrue())
			//
			//c, err := controller.New("foo", m, controller.manager.Options{Reconcile: rec})
			//Expect(err).NotTo(HaveOccurred())
			//ctrl, ok := c.(*internalcontroller.Controller)
			//Expect(ok).To(BeTrue())
			//
			//// Wait for Controller to start
			//Eventually(func() bool { return ctrl.Started }).Should(BeTrue())
		})

		It("should set Fields", func(done Done) {
			m, err := New(cfg, Options{})
			Expect(err).NotTo(HaveOccurred())
			mrg, ok := m.(*controllerManager)
			Expect(ok).To(BeTrue())

			mrg.cache = &informertest.FakeInformers{}

			By("Injecting the dependencies")
			err = m.SetFields(&injectable{
				scheme: func(scheme *runtime.Scheme) error {
					defer GinkgoRecover()
					Expect(scheme).To(Equal(m.GetScheme()))
					return nil
				},
				config: func(config *rest.Config) error {
					defer GinkgoRecover()
					Expect(config).To(Equal(m.GetConfig()))
					return nil
				},
				client: func(client client.Client) error {
					defer GinkgoRecover()
					Expect(client).To(Equal(m.GetClient()))
					return nil
				},
				cache: func(c cache.Cache) error {
					defer GinkgoRecover()
					Expect(c).To(Equal(m.GetCache()))
					return nil
				},
				f: func(f inject.Func) error {
					defer GinkgoRecover()
					Expect(f).NotTo(BeNil())
					return nil
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Returning an error if dependency injection fails")

			expected := fmt.Errorf("expected error")
			err = m.SetFields(&injectable{
				client: func(client client.Client) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				scheme: func(scheme *runtime.Scheme) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				config: func(config *rest.Config) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				cache: func(c cache.Cache) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))

			err = m.SetFields(&injectable{
				f: func(c inject.Func) error {
					return expected
				},
			})
			Expect(err).To(Equal(expected))
			close(done)
		})
	})
})

var _ reconcile.Reconcile = &failRec{}
var _ inject.Client = &failRec{}

type failRec struct{}

func (*failRec) Reconcile(reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (*failRec) InjectClient(client.Client) error {
	return fmt.Errorf("expected error")
}

var _ inject.Injector = &injectable{}
var _ inject.Cache = &injectable{}
var _ inject.Client = &injectable{}
var _ inject.Scheme = &injectable{}
var _ inject.Config = &injectable{}

type injectable struct {
	scheme func(scheme *runtime.Scheme) error
	client func(client.Client) error
	config func(config *rest.Config) error
	cache  func(cache.Cache) error
	f      func(inject.Func) error
}

func (i *injectable) InjectCache(c cache.Cache) error {
	if i.cache == nil {
		return nil
	}
	return i.cache(c)
}

func (i *injectable) InjectConfig(config *rest.Config) error {
	if i.config == nil {
		return nil
	}
	return i.config(config)
}

func (i *injectable) InjectClient(c client.Client) error {
	if i.client == nil {
		return nil
	}
	return i.client(c)
}

func (i *injectable) InjectScheme(scheme *runtime.Scheme) error {
	if i.scheme == nil {
		return nil
	}
	return i.scheme(scheme)
}

func (i *injectable) InjectFunc(f inject.Func) error {
	if i.f == nil {
		return nil
	}
	return i.f(f)
}

func (i *injectable) Start(<-chan struct{}) error {
	return nil
}
