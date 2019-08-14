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

package apiutil

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
	"golang.org/x/xerrors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// ErrRateLimited is returned by a RESTMapper method if the number of API
// calls has exceeded a limit within a certain time period.
type ErrRateLimited struct {
	// Duration to wait until the next API call can be made.
	Delay time.Duration
}

func (e ErrRateLimited) Error() string {
	return "too many API calls to the RESTMapper within a timeframe"
}

// DelayIfRateLimited returns the delay time until the next API call is
// allowed and true if err is of type ErrRateLimited. The zero
// time.Duration value and false are returned if err is not a ErrRateLimited.
func DelayIfRateLimited(err error) (time.Duration, bool) {
	var rlerr ErrRateLimited
	if xerrors.As(err, &rlerr) {
		return rlerr.Delay, true
	}
	return 0, false
}

// dynamicRESTMapper is a RESTMapper that dynamically discovers resource
// types at runtime.
type dynamicRESTMapper struct {
	mu           sync.RWMutex // protects the following fields
	client       discovery.DiscoveryInterface
	staticMapper meta.RESTMapper
	limiter      *dynamicLimiter

	lazy bool
	// Used for lazy init.
	initOnce sync.Once
}

// WithLimiter sets the RESTMapper's underlying limiter to lim.
func WithLimiter(lim *rate.Limiter) func(*dynamicRESTMapper) error {
	return func(drm *dynamicRESTMapper) error {
		drm.limiter = &dynamicLimiter{lim}
		return nil
	}
}

// WithLazyDiscovery prevents the RESTMapper from discovering REST mappings
// until an API call is made.
var WithLazyDiscovery = func(drm *dynamicRESTMapper) error {
	drm.lazy = true
	return nil
}

// NewDynamicRESTMapper returns a dynamic RESTMapper for cfg. The dynamic
// RESTMapper dynamically discovers resource types at runtime. opts
// configure the RESTMapper.
func NewDynamicRESTMapper(cfg *rest.Config, opts ...func(*dynamicRESTMapper) error) (meta.RESTMapper, error) {
	client, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	drm := &dynamicRESTMapper{
		client: client,
		limiter: &dynamicLimiter{
			rate.NewLimiter(rate.Limit(defaultLimitRate), defaultLimitSize),
		},
	}
	for _, opt := range opts {
		if err = opt(drm); err != nil {
			return nil, err
		}
	}
	if !drm.lazy {
		if err := drm.setStaticMapper(); err != nil {
			return nil, err
		}
	}
	return drm, nil
}

var (
	// defaultLimitRate is the number of RESTMapper API calls allowed
	// per second assuming the rate of API calls <= defaultLimitRate.
	defaultLimitRate = 600
	// defaultLimitSize is the maximum number of simultaneous RESTMapper
	// API calls allowed.
	defaultLimitSize = 5
)

// setStaticMapper sets drm's staticMapper by querying its client, regardless
// of reload backoff.
func (drm *dynamicRESTMapper) setStaticMapper() error {
	groupResources, err := restmapper.GetAPIGroupResources(drm.client)
	if err != nil {
		return err
	}
	drm.staticMapper = restmapper.NewDiscoveryRESTMapper(groupResources)
	return nil
}

// init initializes drm only once if drm is lazy.
func (drm *dynamicRESTMapper) init() (err error) {
	drm.initOnce.Do(func() {
		if drm.lazy {
			err = drm.setStaticMapper()
		}
	})
	return err
}

// reload reloads the static RESTMapper, and will return an error only
// if a rate limit has been hit. reload is thread-safe.
func (drm *dynamicRESTMapper) reload() error {
	// limiter is thread-safe.
	if err := drm.limiter.checkRate(); err != nil {
		return err
	}
	// Lock here so callers can be rate-limited regardless of lock state.
	drm.mu.Lock()
	defer drm.mu.Unlock()
	return drm.setStaticMapper()
}

// TODO: wrap reload errors on NoKindMatchError with go 1.13 errors.

func (drm *dynamicRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	if err := drm.init(); err != nil {
		return schema.GroupVersionKind{}, err
	}
	gvk, err := drm.kindFor(resource)
	if xerrors.Is(err, &meta.NoKindMatchError{}) {
		if rerr := drm.reload(); rerr != nil {
			return schema.GroupVersionKind{}, rerr
		}
		gvk, err = drm.kindFor(resource)
	}
	return gvk, err
}

// kindFor calls the underlying static RESTMapper's KindFor method in a
// thread-safe manner.
func (drm *dynamicRESTMapper) kindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.staticMapper.KindFor(resource)
}

func (drm *dynamicRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	gvks, err := drm.kindsFor(resource)
	if xerrors.Is(err, &meta.NoKindMatchError{}) {
		if rerr := drm.reload(); rerr != nil {
			return nil, rerr
		}
		gvks, err = drm.kindsFor(resource)
	}
	return gvks, err
}

// kindsFor calls the underlying static RESTMapper's KindsFor method in a
// thread-safe manner.
func (drm *dynamicRESTMapper) kindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.staticMapper.KindsFor(resource)
}

func (drm *dynamicRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	if err := drm.init(); err != nil {
		return schema.GroupVersionResource{}, err
	}
	gvr, err := drm.resourceFor(input)
	if xerrors.Is(err, &meta.NoKindMatchError{}) {
		if rerr := drm.reload(); rerr != nil {
			return schema.GroupVersionResource{}, rerr
		}
		gvr, err = drm.resourceFor(input)
	}
	return gvr, err
}

// resourceFor calls the underlying static RESTMapper's ResourceFor method in a
// thread-safe manner.
func (drm *dynamicRESTMapper) resourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.staticMapper.ResourceFor(input)
}

func (drm *dynamicRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	gvrs, err := drm.resourcesFor(input)
	if xerrors.Is(err, &meta.NoKindMatchError{}) {
		if rerr := drm.reload(); rerr != nil {
			return nil, rerr
		}
		gvrs, err = drm.resourcesFor(input)
	}
	return gvrs, err
}

// resourcesFor calls the underlying static RESTMapper's ResourcesFor method
// in a thread-safe manner.
func (drm *dynamicRESTMapper) resourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.staticMapper.ResourcesFor(input)
}

func (drm *dynamicRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	mapping, err := drm.restMapping(gk, versions...)
	if xerrors.Is(err, &meta.NoKindMatchError{}) {
		if rerr := drm.reload(); rerr != nil {
			return nil, rerr
		}
		mapping, err = drm.restMapping(gk, versions...)
	}
	return mapping, err
}

// restMapping calls the underlying static RESTMapper's RESTMapping method
// in a thread-safe manner.
func (drm *dynamicRESTMapper) restMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.staticMapper.RESTMapping(gk, versions...)
}

func (drm *dynamicRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	mappings, err := drm.restMappings(gk, versions...)
	if xerrors.Is(err, &meta.NoKindMatchError{}) {
		if rerr := drm.reload(); rerr != nil {
			return nil, rerr
		}
		mappings, err = drm.restMappings(gk, versions...)
	}
	return mappings, err
}

// restMappings calls the underlying static RESTMapper's RESTMappings method
// in a thread-safe manner.
func (drm *dynamicRESTMapper) restMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.staticMapper.RESTMappings(gk, versions...)
}

func (drm *dynamicRESTMapper) ResourceSingularizer(resource string) (string, error) {
	if err := drm.init(); err != nil {
		return "", err
	}
	singular, err := drm.resourceSingularizer(resource)
	if xerrors.Is(err, &meta.NoKindMatchError{}) {
		if rerr := drm.reload(); rerr != nil {
			return "", rerr
		}
		singular, err = drm.resourceSingularizer(resource)
	}
	return singular, err
}

// resourceSingularizer calls the underlying static RESTMapper's
// ResourceSingularizer method in a thread-safe manner.
func (drm *dynamicRESTMapper) resourceSingularizer(resource string) (string, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.staticMapper.ResourceSingularizer(resource)
}

// dynamicLimiter holds a rate limiter used to throttle chatty RESTMapper users.
type dynamicLimiter struct {
	*rate.Limiter
}

// checkRate returns an ErrRateLimited if too many API calls have been made
// within the set limit.
func (b *dynamicLimiter) checkRate() error {
	res := b.Reserve()
	if res.Delay() == 0 {
		return nil
	}
	return ErrRateLimited{res.Delay()}
}
