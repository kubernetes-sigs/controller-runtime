/*
Copyright 2026 The Kubernetes Authors.

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

package legacyrestmapper

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RESTMapper is a RESTMapper that implements both the context-aware
// meta.RESTMapperWithContext and the legacy meta.RESTMapper interface.
//
// The methods of meta.RESTMapperWithContext should be preferred because they support
// cancellation and contextual logging. The legacy methods are copied from meta.RESTMapper
// and use context.Background().
type RESTMapper interface {
	meta.RESTMapperWithContext

	// The following methods are copied from meta.RESTMapper so we can mark them as deprecated.

	// KindFor takes a partial resource and returns the single match.  Returns an error if there are multiple matches
	//
	// Deprecated: Use KindForWithContext instead, it supports cancellation and contextual logging.
	KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error)

	// KindsFor takes a partial resource and returns the list of potential kinds in priority order
	//
	// Deprecated: Use KindsForWithContext instead, it supports cancellation and contextual logging.
	KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error)

	// ResourceFor takes a partial resource and returns the single match.  Returns an error if there are multiple matches
	//
	// Deprecated: Use ResourceForWithContext instead, it supports cancellation and contextual logging.
	ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error)

	// ResourcesFor takes a partial resource and returns the list of potential resource in priority order
	//
	// Deprecated: Use ResourcesForWithContext instead, it supports cancellation and contextual logging.
	ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error)

	// RESTMapping identifies a preferred resource mapping for the provided group kind.
	//
	// Deprecated: Use RESTMappingWithContext instead, it supports cancellation and contextual logging.
	RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)

	// RESTMappings returns all resource mappings for the provided group kind if no
	// version search is provided. Otherwise identifies a preferred resource mapping for
	// the provided version(s).
	//
	// Deprecated: Use RESTMappingsWithContext instead, it supports cancellation and contextual logging.
	RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error)

	// ResourceSingularizer converts a resource name from plural to singular (e.g., from pods to pod).
	//
	// Deprecated: Use ResourceSingularizerWithContext instead, it supports cancellation and contextual logging.
	ResourceSingularizer(resource string) (singular string, err error)
}

// GroupGeneration is an optional interface implemented by RESTMappers
// that track cache invalidations and reloads per API group via a monotonically
// increasing generation.
type GroupGeneration interface {
	GroupGeneration(group string) uint64
}

// GroupInvalidator is an optional interface implemented by RESTMappers
// that support invalidating cached mappings for a single API group.
type GroupInvalidator interface {
	InvalidateGroup(group string)
}

var (
	// RESTMapper must stay assignable to both upstream interfaces.
	_ meta.RESTMapper            = RESTMapper(nil)
	_ meta.RESTMapperWithContext = RESTMapper(nil)

	// wrapper must implement RESTMapper.
	_ RESTMapper = &wrapper{}
)

// Wrap returns a RESTMapper that implements the context-aware methods
// by delegating to m and the legacy methods (without context) by delegating to the
// corresponding context-aware methods of m with context.Background().
//
// If m already is a RESTMapper it is returned as is.
func Wrap(m meta.RESTMapperWithContext) RESTMapper {
	if m == nil {
		return nil
	}
	if d, ok := m.(RESTMapper); ok {
		return d
	}
	return &wrapper{RESTMapperWithContext: m}
}

// wrapper adds the legacy meta.RESTMapper methods on top of a meta.RESTMapperWithContext.
type wrapper struct {
	meta.RESTMapperWithContext
}

// KindFor implements meta.RESTMapper.KindFor.
func (m *wrapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.KindForWithContext(context.Background(), resource)
}

// KindsFor implements meta.RESTMapper.KindsFor.
func (m *wrapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.KindsForWithContext(context.Background(), resource)
}

// ResourceFor implements meta.RESTMapper.ResourceFor.
func (m *wrapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.ResourceForWithContext(context.Background(), input)
}

// ResourcesFor implements meta.RESTMapper.ResourcesFor.
func (m *wrapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return m.ResourcesForWithContext(context.Background(), input)
}

// RESTMapping implements meta.RESTMapper.RESTMapping.
func (m *wrapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.RESTMappingWithContext(context.Background(), gk, versions...)
}

// RESTMappings implements meta.RESTMapper.RESTMappings.
func (m *wrapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.RESTMappingsWithContext(context.Background(), gk, versions...)
}

// ResourceSingularizer implements meta.RESTMapper.ResourceSingularizer.
func (m *wrapper) ResourceSingularizer(resource string) (string, error) {
	return m.ResourceSingularizerWithContext(context.Background(), resource)
}

// Reset implements meta.ResettableRESTMapper.Reset.
func (m *wrapper) Reset() {
	m.ResetWithContext(context.Background())
}

// ResetWithContext implements meta.ResettableRESTMapperWithContext.ResetWithContext.
func (m *wrapper) ResetWithContext(ctx context.Context) {
	if r, ok := m.RESTMapperWithContext.(meta.ResettableRESTMapperWithContext); ok {
		r.ResetWithContext(ctx)
		return
	}
	if r, ok := m.RESTMapperWithContext.(meta.ResettableRESTMapper); ok {
		r.Reset()
	}
}

// InvalidateGroup implements GroupInvalidator.InvalidateGroup.
func (m *wrapper) InvalidateGroup(group string) {
	if inv, ok := m.RESTMapperWithContext.(GroupInvalidator); ok {
		inv.InvalidateGroup(group)
	}
}

// GroupGeneration implements GroupGeneration.GroupGeneration.
func (m *wrapper) GroupGeneration(group string) uint64 {
	if g, ok := m.RESTMapperWithContext.(GroupGeneration); ok {
		return g.GroupGeneration(group)
	}
	return 0
}
