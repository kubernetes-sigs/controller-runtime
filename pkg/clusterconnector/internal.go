/*
Copyright 2020 The Kubernetes Authors.

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

package clusterconnector

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/recorder"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

var _ ClusterConnector = &clusterConnector{}

type clusterConnector struct {
	// config is the rest.config used to talk to the apiserver.  Required.
	config *rest.Config

	// scheme is the scheme injected into Controllers, EventHandlers, Sources and Predicates.  Defaults
	// to scheme.scheme.
	scheme *runtime.Scheme

	cache cache.Cache

	// TODO(directxman12): Provide an escape hatch to get individual indexers
	// client is the client injected into Controllers (and EventHandlers, Sources and Predicates).
	client client.Client

	// apiReader is the reader that will make requests to the api server and not the cache.
	apiReader client.Reader

	// fieldIndexes knows how to add field indexes over the Cache used by this controller,
	// which can later be consumed via field selectors from the injected client.
	fieldIndexes client.FieldIndexer

	// recorderProvider is used to generate event recorders that will be injected into Controllers
	// (and EventHandlers, Sources and Predicates).
	recorderProvider recorder.Provider

	// mapper is used to map resources to kind, and map kind and version.
	mapper meta.RESTMapper
}

func (cc *clusterConnector) SetFields(i interface{}) error {
	if _, err := inject.ConfigInto(cc.config, i); err != nil {
		return err
	}
	if _, err := inject.ClientInto(cc.client, i); err != nil {
		return err
	}
	if _, err := inject.APIReaderInto(cc.apiReader, i); err != nil {
		return err
	}
	if _, err := inject.SchemeInto(cc.scheme, i); err != nil {
		return err
	}
	if _, err := inject.CacheInto(cc.cache, i); err != nil {
		return err
	}
	if _, err := inject.InjectorInto(cc.SetFields, i); err != nil {
		return err
	}
	if _, err := inject.MapperInto(cc.mapper, i); err != nil {
		return err
	}
	return nil
}

func (cc *clusterConnector) GetConfig() *rest.Config {
	return cc.config
}

func (cc *clusterConnector) GetClient() client.Client {
	return cc.client
}

func (cc *clusterConnector) GetScheme() *runtime.Scheme {
	return cc.scheme
}

func (cc *clusterConnector) GetFieldIndexer() client.FieldIndexer {
	return cc.fieldIndexes
}

func (cc *clusterConnector) GetCache() cache.Cache {
	return cc.cache
}

func (cc *clusterConnector) GetEventRecorderFor(name string) record.EventRecorder {
	return cc.recorderProvider.GetEventRecorderFor(name)
}

func (cc *clusterConnector) GetRESTMapper() meta.RESTMapper {
	return cc.mapper
}

func (cc *clusterConnector) GetAPIReader() client.Reader {
	return cc.apiReader
}

func (cc *clusterConnector) AddToManager(mgr Manager) error {
	return mgr.Add(cc.GetCache())
}
