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

package webhook

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/internal/testing/addr"
	fakecertwatcher "sigs.k8s.io/controller-runtime/pkg/internal/testing/certwatcher"
)

var _ = Describe("Webhook Server certificate watcher", func() {
	It("returns an error when the certificate watcher fails", func(specCtx SpecContext) {
		ctx, cancel := context.WithCancel(specCtx)
		defer cancel()

		port, host, err := addr.Suggest("")
		Expect(err).NotTo(HaveOccurred())

		watcherErr := errors.New("certificate watcher boom")
		srv := &DefaultServer{
			Options: Options{Host: host, Port: port},
			newCertWatcher: func(string, string) (certWatcher, error) {
				return &fakecertwatcher.Fake{StartErr: watcherErr}, nil
			},
		}

		errCh := make(chan error, 1)
		go func() {
			defer GinkgoRecover()
			errCh <- srv.Start(ctx)
		}()

		Eventually(errCh, 5*time.Second).Should(Receive(MatchError(watcherErr)))
	})
})
