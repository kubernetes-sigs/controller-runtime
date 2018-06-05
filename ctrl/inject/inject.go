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

package inject

import (
	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/client-go/rest"
)

type IndexInformerCache interface {
	InjectIndexInformerCache(informer.Informers)
}

func InjectInformers(c informer.Informers, i interface{}) bool {
	if s, ok := i.(IndexInformerCache); ok {
		s.InjectIndexInformerCache(c)
		return true
	}
	return false
}

type Config interface {
	InjectConfig(*rest.Config)
}

func InjectConfig(c *rest.Config, i interface{}) bool {
	if s, ok := i.(Config); ok {
		s.InjectConfig(c)
		return true
	}
	return false
}

// TODO(pwittrock): Figure out if this is the pattern we want to stick with
func NewConfig() (*rest.Config, error) {
	return config.GetConfig()
}

type Cache interface {
	InjectCache(*rest.Config)
}

func InjectCache(c *rest.Config, i interface{}) bool {
	if s, ok := i.(Config); ok {
		s.InjectConfig(c)
		return true
	}
	return false
}
