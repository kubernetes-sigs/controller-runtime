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

// Package apiutil contains utilities for working with raw Kubernetes
// API machinery, such as creating RESTMappers and raw REST clients,
// and extracting the GVK of an object.
package apiutil

import (
	"fmt"
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

var (
	protobufScheme     = runtime.NewScheme()
	protobufSchemeLock sync.RWMutex
)

// AddToProtobufScheme add the given SchemeBuilder into protobufScheme, which should
// be additional types where we do want to support protobuf.
func AddToProtobufScheme(addToScheme func(*runtime.Scheme) error) error {
	protobufSchemeLock.Lock()
	defer protobufSchemeLock.Unlock()
	return addToScheme(protobufScheme)
}

// NewDiscoveryRESTMapper constructs a new RESTMapper based on discovery
// information fetched by a new client with the given config.
func NewDiscoveryRESTMapper(c *rest.Config) (meta.RESTMapper, error) {
	// Get a mapper
	dc, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return nil, err
	}
	gr, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return nil, err
	}
	return restmapper.NewDiscoveryRESTMapper(gr), nil
}

// GVKForObject finds the GroupVersionKind associated with the given object, if there is only a single such GVK.
func GVKForObject(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	// TODO(directxman12): do we want to generalize this to arbitrary container types?
	// I think we'd need a generalized form of scheme or something.  It's a
	// shame there's not a reliable "GetGVK" interface that works by default
	// for unpopulated static types and populated "dynamic" types
	// (unstructured, partial, etc)

	// check for PartialObjectMetadata, which is analogous to unstructured, but isn't handled by ObjectKinds
	_, isPartial := obj.(*metav1.PartialObjectMetadata) //nolint:ifshort
	_, isPartialList := obj.(*metav1.PartialObjectMetadataList)
	if isPartial || isPartialList {
		// we require that the GVK be populated in order to recognize the object
		gvk := obj.GetObjectKind().GroupVersionKind()
		if len(gvk.Kind) == 0 {
			return schema.GroupVersionKind{}, runtime.NewMissingKindErr("unstructured object has no kind")
		}
		if len(gvk.Version) == 0 {
			return schema.GroupVersionKind{}, runtime.NewMissingVersionErr("unstructured object has no version")
		}
		return gvk, nil
	}

	gvks, isUnversioned, err := scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	if isUnversioned {
		return schema.GroupVersionKind{}, fmt.Errorf("cannot create group-version-kind for unversioned type %T", obj)
	}

	if len(gvks) < 1 {
		return schema.GroupVersionKind{}, fmt.Errorf("no group-version-kinds associated with type %T", obj)
	}
	if len(gvks) > 1 {
		// this should only trigger for things like metav1.XYZ --
		// normal versioned types should be fine
		return schema.GroupVersionKind{}, fmt.Errorf(
			"multiple group-version-kinds associated with type %T, refusing to guess at one", obj)
	}
	return gvks[0], nil
}

// RESTClientForGVK constructs a new rest.Interface capable of accessing the resource associated
// with the given GroupVersionKind. The REST client will be configured to use the negotiated serializer from
// baseConfig, if set, otherwise a default serializer will be set.
func RESTClientForGVK(scheme *runtime.Scheme, gvk schema.GroupVersionKind, isUnstructured bool, baseConfig *rest.Config, codecs serializer.CodecFactory) (rest.Interface, error) {
	return rest.RESTClientFor(createRestConfig(scheme, gvk, isUnstructured, baseConfig, codecs))
}

// serializerWithDecodedGVK is a CodecFactory that overrides the DecoderToVersion of a WithoutConversionCodecFactory
// in order to avoid clearing the GVK from the decoded object.
//
// See https://github.com/kubernetes/kubernetes/issues/80609.
type serializerWithDecodedGVK struct {
	serializer.WithoutConversionCodecFactory
}

// DecoderToVersion returns an decoder that does not do conversion.
func (f serializerWithDecodedGVK) DecoderToVersion(serializer runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return serializer
}

// createRestConfig copies the base config and updates needed fields for a new rest config.
func createRestConfig(scheme *runtime.Scheme, gvk schema.GroupVersionKind, isUnstructured bool, baseConfig *rest.Config, codecs serializer.CodecFactory) *rest.Config {
	gv := gvk.GroupVersion()

	cfg := rest.CopyConfig(baseConfig)
	cfg.GroupVersion = &gv
	if gvk.Group == "" {
		cfg.APIPath = "/api"
	} else {
		cfg.APIPath = "/apis"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	// TODO(FillZpp): In the long run, we want to check discovery or something to make sure that this is actually true.
	if cfg.ContentType == "" && !isUnstructured {
		if canUseProtobuf(scheme, gvk) {
			cfg.ContentType = runtime.ContentTypeProtobuf
		}
	}

	if isUnstructured {
		// If the object is unstructured, we need to preserve the GVK information.
		// Use our own custom serializer.
		cfg.NegotiatedSerializer = serializerWithDecodedGVK{serializer.WithoutConversionCodecFactory{CodecFactory: codecs}}
	} else {
		cfg.NegotiatedSerializer = serializerWithTargetZeroingDecode{NegotiatedSerializer: serializer.WithoutConversionCodecFactory{CodecFactory: codecs}}
	}

	return cfg
}

type serializerWithTargetZeroingDecode struct {
	runtime.NegotiatedSerializer
}

func (s serializerWithTargetZeroingDecode) DecoderToVersion(serializer runtime.Decoder, r runtime.GroupVersioner) runtime.Decoder {
	return targetZeroingDecoder{upstream: s.NegotiatedSerializer.DecoderToVersion(serializer, r)}
}

type targetZeroingDecoder struct {
	upstream runtime.Decoder
}

func (t targetZeroingDecoder) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	zero(into)
	return t.upstream.Decode(data, defaults, into)
}

// zero zeros the value of a pointer.
func zero(x interface{}) {
	if x == nil {
		return
	}
	res := reflect.ValueOf(x).Elem()
	res.Set(reflect.Zero(res.Type()))
}

// canUseProtobuf returns true if we should use protobuf encoding.
// We need two things: (1) the apiserver must support protobuf for the type,
// and (2) we must have a proto-compatible receiving go type.
// Because it's hard to know in general if apiserver supports proto for a given GVK,
// we maintain a list of built-in apigroups which do support proto;
// additional schemas can be added as proto-safe using AddToProtobufScheme.
// We check if we have a proto-compatible type by interface casting.
func canUseProtobuf(scheme *runtime.Scheme, gvk schema.GroupVersionKind) bool {
	// Check that the client supports proto for this type
	gvkType, found := scheme.AllKnownTypes()[gvk]
	if !found {
		// We aren't going to be able to deserialize proto without type information.
		return false
	}
	if !implementsProto(gvkType) {
		// We don't have proto information, can't parse proto
		return false
	}

	// Check that the apiserver also supports proto for this type
	serverSupportsProto := false

	// Check for built-in types well-known to support proto
	serverSupportsProto = isWellKnownKindThatSupportsProto(gvk)

	// Check for additional types explicitly marked as supporting proto
	if !serverSupportsProto {
		protobufSchemeLock.RLock()
		serverSupportsProto = protobufScheme.Recognizes(gvk)
		protobufSchemeLock.RUnlock()
	}

	return serverSupportsProto
}

// isWellKnownKindThatSupportsProto returns true if the gvk is a well-known Kind that supports proto encoding.
func isWellKnownKindThatSupportsProto(gvk schema.GroupVersionKind) bool {
	// corev1
	if gvk.Group == "" && gvk.Version == "v1" {
		return true
	}

	// extensions v1beta1
	if gvk.Group == "extensions" && gvk.Version == "v1beta1" {
		return true
	}

	// Generated with `kubectl api-resources -oname | grep "\." | sort | cut -f2- -d. | sort | uniq | awk '{print "case \"" $0 "\": return true" }'` (before adding any CRDs)
	switch gvk.Group {
	case "admissionregistration.k8s.io":
		return true
	case "apiextensions.k8s.io":
		return true
	case "apiregistration.k8s.io":
		return true
	case "apps":
		return true
	case "authentication.k8s.io":
		return true
	case "authorization.k8s.io":
		return true
	case "autoscaling":
		return true
	case "batch":
		return true
	case "certificates.k8s.io":
		return true
	case "coordination.k8s.io":
		return true
	case "discovery.k8s.io":
		return true
	case "events.k8s.io":
		return true
	case "flowcontrol.apiserver.k8s.io":
		return true
	case "networking.k8s.io":
		return true
	case "node.k8s.io":
		return true
	case "policy":
		return true
	case "rbac.authorization.k8s.io":
		return true
	case "scheduling.k8s.io":
		return true
	case "storage.k8s.io":
		return true
	}
	return false
}

var (
	memoizeImplementsProto     = make(map[reflect.Type]bool)
	memoizeImplementsProtoLock sync.RWMutex
)

// protoMessage is implemented by protobuf messages (of all libraries).
type protoMessage interface {
	ProtoMessage()
}

var protoMessageType = reflect.TypeOf(new(protoMessage)).Elem()

// implementsProto returns true if the local go type supports protobuf deserialization.
func implementsProto(t reflect.Type) bool {
	memoizeImplementsProtoLock.RLock()
	result, found := memoizeImplementsProto[t]
	memoizeImplementsProtoLock.RUnlock()

	if found {
		return result
	}

	// We normally get the raw struct e.g. v1.Pod, not &v1.Pod
	if t.Kind() == reflect.Struct {
		return implementsProto(reflect.PtrTo(t))
	}

	result = t.Implements(protoMessageType)
	memoizeImplementsProtoLock.Lock()
	memoizeImplementsProto[t] = result
	memoizeImplementsProtoLock.Unlock()

	return result
}
