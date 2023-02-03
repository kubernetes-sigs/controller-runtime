package komega

import (
	"fmt"

	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HaveJSONPathMatcher struct {
	jsonpath string
	matcher  types.GomegaMatcher
	name     string
	value    interface{}
}

func HaveJSONPath(jsonpath string, matcher types.GomegaMatcher) *HaveJSONPathMatcher {
	return &HaveJSONPathMatcher{
		jsonpath: jsonpath,
		matcher:  matcher,
	}
}

func (m *HaveJSONPathMatcher) Match(actual interface{}) (success bool, err error) {
	j := jsonpath.New("")
	if err := j.Parse(m.jsonpath); err != nil {
		return false, fmt.Errorf("JSON Path '%s' is invalid: %s", m.jsonpath, err.Error())
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
		return false, fmt.Errorf("JSON Path '%s' failed: %s", m.jsonpath, err.Error())
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

func (m *HaveJSONPathMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s at %s: %s", m.name, m.jsonpath, m.matcher.FailureMessage(m.value))
}

func (m *HaveJSONPathMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s at %s: %s", m.name, m.jsonpath, m.matcher.NegatedFailureMessage(m.value))
}
