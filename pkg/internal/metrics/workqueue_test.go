/*
Copyright The Kubernetes Authors.

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

package metrics

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/sets"
)

type fakeGauge struct {
	prometheus.Gauge
	inc func()
}

func (f *fakeGauge) Inc() { f.inc() }

type fakeGaugeVec struct {
	gauges map[string]int
	mu     sync.Mutex
}

func (f *fakeGaugeVec) WithLabelValues(lvs ...string) prometheus.Gauge {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := ""
	for _, lv := range lvs {
		key += lv + "|"
	}
	if _, ok := f.gauges[key]; !ok {
		f.gauges[key] = 0
	}
	return &fakeGauge{inc: func() {
		f.mu.Lock()
		f.gauges[key]++
		f.mu.Unlock()
	}}
}

var _ = Describe("depthWithPriorityMetric", func() {
	Describe("CardinalityLimit", func() {
		type testCase struct {
			numUniquePriorities            int
			expectedKeyCount               int
			expectedCardinalityExceededVal int
		}

		DescribeTable("should respect cardinality limits",
			func(tc testCase) {
				fakeVec := &fakeGaugeVec{gauges: make(map[string]int)}
				m := &depthWithPriorityMetric{
					depth:              fakeVec,
					lvs:                []string{"test", "test"},
					observedPriorities: sets.Set[int]{},
				}

				wg := &sync.WaitGroup{}
				wg.Add(tc.numUniquePriorities)
				for i := 1; i <= tc.numUniquePriorities; i++ {
					go func() {
						m.Inc(i)
						wg.Done()
					}()
				}
				wg.Wait()

				Expect(fakeVec.gauges).To(HaveLen(tc.expectedKeyCount))

				placeholderKey := "test|test|" + priorityCardinalityExceededPlaceholder + "|"
				Expect(fakeVec.gauges[placeholderKey]).To(Equal(tc.expectedCardinalityExceededVal))
			},
			Entry("under limit does not use placeholder", testCase{
				numUniquePriorities:            10,
				expectedKeyCount:               10,
				expectedCardinalityExceededVal: 0,
			}),
			Entry("at limit does not use placeholder", testCase{
				numUniquePriorities:            25,
				expectedKeyCount:               25,
				expectedCardinalityExceededVal: 0,
			}),
			Entry("exceeding limit uses placeholder", testCase{
				numUniquePriorities:            26,
				expectedKeyCount:               26,
				expectedCardinalityExceededVal: 1,
			}),
			Entry("well over limit uses placeholder for all excess", testCase{
				numUniquePriorities:            30,
				expectedKeyCount:               26,
				expectedCardinalityExceededVal: 5,
			}),
		)
	})

	It("same priority many adds does not trigger cardinality limit", func() {
		fakeVec := &fakeGaugeVec{gauges: make(map[string]int)}
		m := &depthWithPriorityMetric{
			depth:              fakeVec,
			lvs:                []string{"test", "test"},
			observedPriorities: sets.Set[int]{},
		}

		wg := &sync.WaitGroup{}
		wg.Add(200)
		for range 200 {
			go func() {
				m.Inc(1)
				wg.Done()
			}()
		}
		wg.Wait()

		Expect(fakeVec.gauges).To(HaveLen(1))
		Expect(fakeVec.gauges["test|test|1|"]).To(Equal(200))
	})
})
