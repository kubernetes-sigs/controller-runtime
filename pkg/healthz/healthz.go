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

package healthz

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

type Handler struct {
	checks []Checker
}

// GetChecks returns all health checks.
func (h *Handler) GetChecks() []Checker {
	return h.checks
}

// AddCheck adds new health check to handler.
func (h *Handler) AddCheck(check Checker) {
	h.checks = append(h.checks, check)
}

// Build creates http.Handler that serves checks.
func (h *Handler) Build() http.Handler {
	return handleRootHealthz(h.checks...)
}

// HealthzChecker is a named healthz checker.
type Checker interface {
	Name() string
	Check(req *http.Request) error
}

// PingHealthz returns true automatically when checked
var PingHealthz Checker = ping{}

// ping implements the simplest possible healthz checker.
type ping struct{}

func (ping) Name() string {
	return "ping"
}

// PingHealthz is a health check that returns true.
func (ping) Check(_ *http.Request) error {
	return nil
}

// NamedCheck returns a healthz checker for the given name and function.
func NamedCheck(name string, check func(r *http.Request) error) Checker {
	return &healthzCheck{name, check}
}

// InstallPathHandler registers handlers for health checking on
// a specific path to mux. *All handlers* for the path must be
// specified in exactly one call to InstallPathHandler. Calling
// InstallPathHandler more than once for the same path and mux will
// result in a panic.
func InstallPathHandler(mux mux, path string, handler *Handler) {
	if len(handler.GetChecks()) == 0 {
		log.V(1).Info("No default health checks specified. Installing the ping handler.")
		handler.AddCheck(PingHealthz)
	}

	mux.Handle(path, handler.Build())
	for _, check := range handler.GetChecks() {
		log.V(1).Info("installing healthz checker", "checker", check.Name())
		mux.Handle(fmt.Sprintf("%s/%s", path, check.Name()), adaptCheckToHandler(check.Check))
	}
}

// mux is an interface describing the methods InstallHandler requires.
type mux interface {
	Handle(pattern string, handler http.Handler)
}

// healthzCheck implements HealthzChecker on an arbitrary name and check function.
type healthzCheck struct {
	name  string
	check func(r *http.Request) error
}

var _ Checker = &healthzCheck{}

func (c *healthzCheck) Name() string {
	return c.name
}

func (c *healthzCheck) Check(r *http.Request) error {
	return c.check(r)
}

// getExcludedChecks extracts the health check names to be excluded from the query param
func getExcludedChecks(r *http.Request) sets.String {
	checks, found := r.URL.Query()["exclude"]
	if found {
		return sets.NewString(checks...)
	}
	return sets.NewString()
}

// handleRootHealthz returns an http.HandlerFunc that serves the provided checks.
func handleRootHealthz(checks ...Checker) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failed := false
		excluded := getExcludedChecks(r)
		var verboseOut bytes.Buffer
		for _, check := range checks {
			// no-op the check if we've specified we want to exclude the check
			if excluded.Has(check.Name()) {
				excluded.Delete(check.Name())
				fmt.Fprintf(&verboseOut, "[+]%s excluded: ok\n", check.Name())
				continue
			}
			if err := check.Check(r); err != nil {
				// don't include the error since this endpoint is public.  If someone wants more detail
				// they should have explicit permission to the detailed checks.
				log.V(1).Info("healthz check failed", "checker", check.Name(), "error", err)
				fmt.Fprintf(&verboseOut, "[-]%s failed: reason withheld\n", check.Name())
				failed = true
			} else {
				fmt.Fprintf(&verboseOut, "[+]%s ok\n", check.Name())
			}
		}
		if excluded.Len() > 0 {
			fmt.Fprintf(&verboseOut, "warn: some health checks cannot be excluded: no matches for %s\n", formatQuoted(excluded.List()...))
			for _, c := range excluded.List() {
				log.Info("cannot exclude health check, no matches for it", "checker", c)
			}
		}
		// always be verbose on failure
		if failed {
			log.V(1).Info("healthz check failed", "message", verboseOut.String())
			http.Error(w, fmt.Sprintf("%shealthz check failed", verboseOut.String()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if _, found := r.URL.Query()["verbose"]; !found {
			fmt.Fprint(w, "ok")
			return
		}

		_, err := verboseOut.WriteTo(w)
		if err != nil {
			log.V(1).Info("healthz check failed", "message", verboseOut.String())
			http.Error(w, fmt.Sprintf("%shealthz check failed", verboseOut.String()), http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "healthz check passed\n")
	})
}

// adaptCheckToHandler returns an http.HandlerFunc that serves the provided checks.
func adaptCheckToHandler(c func(r *http.Request) error) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := c(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("internal server error: %v", err), http.StatusInternalServerError)
		} else {
			fmt.Fprint(w, "ok")
		}
	})
}

// formatQuoted returns a formatted string of the health check names,
// preserving the order passed in.
func formatQuoted(names ...string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return strings.Join(quoted, ",")
}
