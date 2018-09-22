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

package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ghodss/yaml"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type writerClient struct {
	// needSeparator indicates if we need a separator when printing next object.
	needSeparator bool
	// writer is the destination where the client writes to.
	writer io.Writer
}

func (c *writerClient) printObject(object runtime.Object) error {
	marshaled, err := yaml.Marshal(object)
	if err != nil {
		return fmt.Errorf("unable to marshal object: %#v", object)
	}
	if c.needSeparator {
		_, err = c.writer.Write([]byte("---\n"))
		if err != nil {
			return err
		}
	}
	_, err = c.writer.Write(marshaled)
	return err
}

var _ client.Client = &writerClient{}

func (c *writerClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
}

func (c *writerClient) List(ctx context.Context, opts *client.ListOptions, list runtime.Object) error {
	return errors.New("method List is not implemented")
}

func (c *writerClient) Create(ctx context.Context, obj runtime.Object) error {
	err := c.printObject(obj)
	if err != nil {
		return err
	}
	c.needSeparator = true
	return nil
}

func (c *writerClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOptionFunc) error {
	return nil
}

func (c *writerClient) Update(ctx context.Context, obj runtime.Object) error {
	err := c.printObject(obj)
	if err != nil {
		return err
	}
	c.needSeparator = true
	return nil
}

func (c *writerClient) Status() client.StatusWriter {
	return nil
}
