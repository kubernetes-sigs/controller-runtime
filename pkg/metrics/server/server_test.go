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

package server

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Server Suite")
}

var _ = Describe("handlerOpts", func() {
	type testCase struct {
		server                    defaultServer
		expectedErrorHandling     promhttp.HandlerErrorHandling
		expectedEnableOpenMetrics bool
	}

	DescribeTable("returns the expected handler options",
		func(tc *testCase) {
			opts := tc.server.handlerOpts()

			Expect(opts.ErrorHandling).To(Equal(tc.expectedErrorHandling))
			Expect(opts.EnableOpenMetrics).To(Equal(tc.expectedEnableOpenMetrics))
		},
		Entry("with defaults", &testCase{
			server:                    defaultServer{},
			expectedErrorHandling:     promhttp.HTTPErrorOnError,
			expectedEnableOpenMetrics: false,
		}),
		Entry("with overrides", &testCase{
			server: defaultServer{
				options: Options{
					HandlerOpts: []func(*promhttp.HandlerOpts){
						func(opts *promhttp.HandlerOpts) {
							opts.ErrorHandling = promhttp.ContinueOnError
							opts.EnableOpenMetrics = true
						},
					},
				},
			},
			expectedErrorHandling:     promhttp.ContinueOnError,
			expectedEnableOpenMetrics: true,
		}),
	)
})
