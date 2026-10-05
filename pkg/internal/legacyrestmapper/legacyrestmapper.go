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

// Wrap returns a RESTMapper that implements the context-aware methods
// by delegating to m and the legacy methods (without context) by delegating to the
// corresponding context-aware methods of m with context.Background().
//
// If m already is a *RESTMapper it is returned as is. m must not be nil.
func Wrap(m meta.RESTMapperWithContext) *RESTMapper {
	if d, ok := m.(*RESTMapper); ok {
		return d
	}
	return &RESTMapper{RESTMapperWithContext: m}
}

// RESTMapper adds the legacy meta.RESTMapper methods on top of a meta.RESTMapperWithContext.
// It satisfies apiutil.RESTMapper.
type RESTMapper struct {
	meta.RESTMapperWithContext
}

// KindFor implements meta.RESTMapper.KindFor.
func (m *RESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.KindForWithContext(context.Background(), resource)
}

// KindsFor implements meta.RESTMapper.KindsFor.
func (m *RESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.KindsForWithContext(context.Background(), resource)
}

// ResourceFor implements meta.RESTMapper.ResourceFor.
func (m *RESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.ResourceForWithContext(context.Background(), input)
}

// ResourcesFor implements meta.RESTMapper.ResourcesFor.
func (m *RESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return m.ResourcesForWithContext(context.Background(), input)
}

// RESTMapping implements meta.RESTMapper.RESTMapping.
func (m *RESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.RESTMappingWithContext(context.Background(), gk, versions...)
}

// RESTMappings implements meta.RESTMapper.RESTMappings.
func (m *RESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.RESTMappingsWithContext(context.Background(), gk, versions...)
}

// ResourceSingularizer implements meta.RESTMapper.ResourceSingularizer.
func (m *RESTMapper) ResourceSingularizer(resource string) (string, error) {
	return m.ResourceSingularizerWithContext(context.Background(), resource)
}
