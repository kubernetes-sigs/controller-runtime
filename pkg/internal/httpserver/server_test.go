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

package httpserver

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("New", func() {
	It("should construct a server with default timeouts and settings", func(ctx SpecContext) {
		handler := http.NewServeMux()

		srv := New(ctx, handler)
		Expect(srv).NotTo(BeNil())
		Expect(srv.Handler).To(Equal(handler))
		Expect(srv.MaxHeaderBytes).To(Equal(1 << 20))
		Expect(srv.IdleTimeout).To(Equal(120 * time.Second))
		Expect(srv.ReadHeaderTimeout).To(Equal(32 * time.Second))
	})
})
