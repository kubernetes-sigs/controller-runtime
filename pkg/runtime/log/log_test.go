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

package log

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	tlogr "github.com/thockin/logr/testing"
)

var _ = Describe("runtime log", func() {

	Context("Test Logger", func() {
		It("shoud set and fulfill with logger", func() {
			logger := ZapLogger(false)
			Expect(logger).NotTo(BeNil())
			Log.WithName("runtimeLog").WithTags("newtag", "newvalue")
			SetLogger(logger)
			logger.WithName("runtimeLog").WithTags("newtag", "newvalue")
			Expect(Log.promise).To(BeNil())
			Expect(Log.Logger).To(Equal(logger))
			devLogger := ZapLogger(true)
			Expect(devLogger).NotTo(BeNil())
		})

		It("should delegate with name", func() {
			var name = "NoPromise"
			test := tlogr.NullLogger{}
			test.WithName(name)
			Log = &DelegatingLogger{
				Logger: tlogr.NullLogger{},
			}
			Log.WithName(name)
			Expect(Log.Logger).To(Equal(test))
		})

		It("should delegate with tags", func() {
			tags := []interface{}{"new", "tags"}
			test := tlogr.NullLogger{}
			test.WithTags(tags)
			Log = &DelegatingLogger{
				Logger: tlogr.NullLogger{},
			}
			Log.WithTags(tags)
			Expect(Log.Logger).To(Equal(test))
		})

	})

})
