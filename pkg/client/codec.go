package client

import (
	"errors"
	"net/url"

	"k8s.io/apimachinery/pkg/conversion/queryparams"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type parameterCodec struct{}

func (parameterCodec) EncodeParameters(obj runtime.Object, to schema.GroupVersion) (url.Values, error) {
	return queryparams.Convert(obj)
}

func (parameterCodec) DecodeParameters(parameters url.Values, from schema.GroupVersion, into runtime.Object) error {
	return errors.New("DecodeParameters not implemented on dynamic parameterCodec")
}
