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

package internal

import (
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Informer = filteredInformers{}

type filteredInformers []filteredInformer

type filteredInformer struct {
	informer cache.SharedIndexInformer
	filter   listWatchFilter
}

func (fis filteredInformers) AddEventHandler(handler cache.ResourceEventHandler) {
	for _, fi := range fis {
		fi.informer.AddEventHandler(handler)
	}
}

func (fis filteredInformers) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) {
	for _, fi := range fis {
		fi.informer.AddEventHandlerWithResyncPeriod(handler, resyncPeriod)
	}
}

func (fis filteredInformers) AddIndexers(indexers cache.Indexers) error {
	for _, fi := range fis {
		if err := fi.informer.AddIndexers(indexers); err != nil {
			return err
		}
	}
	return nil
}

func (fis filteredInformers) HasSynced() bool {
	for _, fi := range fis {
		if !fi.informer.HasSynced() {
			return false
		}
	}
	return true
}

type listWatchFilter struct {
	*client.ListOptions
}

func newListWatchFilter(opts []client.ListOption) (f listWatchFilter) {
	f.ListOptions = &client.ListOptions{}
	f.ApplyOptions(opts)
	return f
}

func (f listWatchFilter) toListOptionsModifier() listOptionsModifier {
	return func(opts *metav1.ListOptions) {
		if f.ListOptions == nil {
			return
		}
		if f.LabelSelector != nil {
			opts.LabelSelector = f.LabelSelector.String()
		}
		if f.FieldSelector != nil {
			opts.FieldSelector = f.FieldSelector.String()
		}
		// TODO(estroz): I don't think it is necessary to support limit or continue here.
		// if !opts.Watch {
		// 	opts.Limit = f.Limit
		// 	opts.Continue = f.Continue
		// }
	}
}

func (f listWatchFilter) isEmpty() bool {
	return f.LabelSelector == nil && f.FieldSelector == nil
}

func (f listWatchFilter) equals(m listWatchFilter) bool {
	return reflect.DeepEqual(f, m)
}
