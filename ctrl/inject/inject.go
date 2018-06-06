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
	"github.com/kubernetes-sigs/kubebuilder/pkg/client"
	"github.com/kubernetes-sigs/kubebuilder/pkg/config"
	"github.com/kubernetes-sigs/kubebuilder/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

type Informers interface {
	InjectInformers(informer.Informers)
}

func InjectInformers(c informer.Informers, i interface{}) bool {
	if s, ok := i.(Informers); ok {
		s.InjectInformers(c)
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

type Client interface {
	InjectClient(client.Interface, client.Interface)
}

func InjectClient(cacheClient client.Interface, liveClient client.Interface, i interface{}) bool {
	if s, ok := i.(Client); ok {
		s.InjectClient(cacheClient, liveClient)
		return true
	}
	return false
}

type Scheme interface {
	InjectScheme(scheme *runtime.Scheme)
}

func InjectScheme(s *runtime.Scheme, i interface{}) bool {
	if is, ok := i.(Scheme); ok {
		is.InjectScheme(s)
		return true
	}
	return false
}
