package selector

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

// Selector specify the label/field selector to fill in ListOptions
type Selector struct {
	Label labels.Selector
	Field fields.Selector
}

// FillInListOpts fill in ListOptions LabelSelector and FieldSelector if needed
func (s Selector) FillInListOpts(listOpts *metav1.ListOptions) {
	if s.Label != nil {
		listOpts.LabelSelector = s.Label.String()
	}
	if s.Field != nil {
		listOpts.FieldSelector = s.Field.String()
	}
}
