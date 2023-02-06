package komega

import (
	"fmt"

	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type haveJSONPathMatcher struct {
	jsonpath string
	matcher  types.GomegaMatcher
	name     string
	value    interface{}
}

// HaveJSONPath returns the matcher for a given JSON path and subsequent matcher
func HaveJSONPath(jsonpath string, matcher types.GomegaMatcher) types.GomegaMatcher {
	return &haveJSONPathMatcher{
		jsonpath: jsonpath,
		matcher:  matcher,
	}
}

// Match transforms the current object to a specified JSON path and returns true if the subsequent
// matcher passes
func (m *haveJSONPathMatcher) Match(actual interface{}) (success bool, err error) {
	j := jsonpath.New("")
	if err := j.Parse(m.jsonpath); err != nil {
		return false, fmt.Errorf("JSON Path '%s' is invalid: %w", m.jsonpath, err)
	}

	if o, ok := actual.(client.Object); ok {
		m.name = fmt.Sprintf("%T %s/%s", actual, o.GetNamespace(), o.GetName())
	} else {
		m.name = fmt.Sprintf("%T", actual)
	}

	var obj interface{}
	if u, ok := actual.(*unstructured.Unstructured); ok {
		obj = u.UnstructuredContent()
	} else {
		obj = actual
	}

	results, err := j.FindResults(obj)
	if err != nil {
		return false, fmt.Errorf("JSON Path '%s' failed: %w", m.jsonpath, err)
	}

	values := []interface{}{}
	for i := range results {
		for j := range results[i] {
			values = append(values, results[i][j].Interface())
		}
	}
	m.value = values

	// flatten values if single result
	if len(values) == 1 {
		m.value = values[0]
	}

	return m.matcher.Match(m.value)
}

// FailureMessage returns a message comparing the expected and actual after an unexpected failure to match has occurred.
func (m *haveJSONPathMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s at %s: %s", m.name, m.jsonpath, m.matcher.FailureMessage(m.value))
}

// NegatedFailureMessage returns a message comparing the expected and actual after an unexpected successful match has occurred.
func (m *haveJSONPathMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s at %s: %s", m.name, m.jsonpath, m.matcher.NegatedFailureMessage(m.value))
}
