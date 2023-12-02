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

package signals

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("runtime signal", func() {

	Context("SignalHandler Test", func() {

		It("test signal handler with delay", func() {
			delay := time.Second
			ctx := SetupSignalHandlerWithDelay(delay)

			// Save time before sending signal
			beforeSendingSignal := time.Now()

			// Send signal
			signalCh <- os.Interrupt

			_, ok := <-ctx.Done()
			// Verify that the channel was closed
			Expect(ok).To(BeFalse())
			// Verify that our delay was respected
			Expect(time.Since(beforeSendingSignal)).To(BeNumerically(">=", delay))
		})

	})

})
