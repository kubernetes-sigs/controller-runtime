package internal

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SelectorsByGVK associate a GroupVersionKind to a field/label selector
type SelectorsByGVK map[schema.GroupVersionKind]Selectors

// Selectors specify the label/field selector to fill in ListOptions
type Selectors []client.Selector

// ApplyToList fill in ListOptions LabelSelector and FieldSelector if needed
func (s Selectors) ApplyToList(listOpts *metav1.ListOptions) {
	opts := &client.ListOptions{}
	for _, selector := range s {
		selector.ApplyToList(opts)
	}

	if opts.LabelSelector != nil {
		listOpts.LabelSelector = opts.LabelSelector.String()
	}
	if opts.FieldSelector != nil {
		listOpts.FieldSelector = opts.FieldSelector.String()
	}
}
