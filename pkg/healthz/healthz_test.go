/*
Copyright 2014 The Kubernetes Authors.

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

package healthz_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

const (
	contentType = "text/plain; charset=utf-8"
)

var _ = Describe("Healthz", func() {
	Describe("Install", func() {
		It("should install handler", func(done Done) {
			mux := http.NewServeMux()
			handler := &healthz.Handler{}

			healthz.InstallPathHandler(mux, "/healthz/test", handler)
			req, err := http.NewRequest("GET", "http://example.com/healthz/test", nil)
			Expect(req).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal(contentType))
			Expect(w.Body.String()).To(Equal("ok"))

			close(done)
		})
	})

	Describe("Checks", func() {
		var testMultipleChecks = func(path string) {
			tests := []struct {
				path             string
				expectedResponse string
				expectedStatus   int
				addBadCheck      bool
			}{
				{"?verbose", "[+]ping ok\nhealthz check passed\n", http.StatusOK,
					false},
				{"?exclude=dontexist", "ok", http.StatusOK, false},
				{"?exclude=bad", "ok", http.StatusOK, true},
				{"?verbose=true&exclude=bad", "[+]ping ok\n[+]bad excluded: ok\nhealthz check passed\n",
					http.StatusOK, true},
				{"?verbose=true&exclude=dontexist",
					"[+]ping ok\nwarn: some health checks cannot be excluded: no matches for \"dontexist\"\nhealthz check passed\n",
					http.StatusOK, false},
				{"/ping", "ok", http.StatusOK, false},
				{"", "ok", http.StatusOK, false},
				{"?verbose", "[+]ping ok\n[-]bad failed: reason withheld\nhealthz check failed\n",
					http.StatusInternalServerError, true},
				{"/ping", "ok", http.StatusOK, true},
				{"/bad", "internal server error: this will fail\n",
					http.StatusInternalServerError, true},
				{"", "[+]ping ok\n[-]bad failed: reason withheld\nhealthz check failed\n",
					http.StatusInternalServerError, true},
			}

			for _, test := range tests {
				mux := http.NewServeMux()
				checks := []healthz.Checker{healthz.PingHealthz}
				if test.addBadCheck {
					checks = append(checks, healthz.NamedCheck("bad", func(_ *http.Request) error {
						return errors.New("this will fail")
					}))
				}
				handler := &healthz.Handler{}
				for _, check := range checks {
					handler.AddCheck(check)
				}

				if path == "" {
					path = "/healthz"
				}

				healthz.InstallPathHandler(mux, path, handler)
				req, err := http.NewRequest("GET", fmt.Sprintf("http://example.com%s%v", path, test.path), nil)
				Expect(req).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)
				Expect(w.Code).To(Equal(test.expectedStatus))
				Expect(w.Header().Get("Content-Type")).To(Equal(contentType))
				Expect(w.Body.String()).To(Equal(test.expectedResponse))
			}
		}

		It("should do multiple checks", func(done Done) {
			testMultipleChecks("")
			close(done)
		})

		It("should do multiple path checks", func(done Done) {
			testMultipleChecks("/ready")
			close(done)
		})
	})
})
