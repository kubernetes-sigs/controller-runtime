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

package writer

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

type fakeCertWriter struct {
	hasInvokedEnsureCerts bool
	err                   error
}

var _ CertWriter = &fakeCertWriter{}

func (f *fakeCertWriter) EnsureCerts(webhookConfig runtime.Object) error {
	f.hasInvokedEnsureCerts = true
	if f.err != nil {
		return f.err
	}
	return nil
}

var _ = Describe("MultipleCertWriter", func() {
	It("should invoke Provide method of each Provider", func() {
		webhookConfig := &admissionregistration.MutatingWebhookConfiguration{}
		f := &fakeCertWriter{}
		m := &MultiCertWriter{
			CertWriters: []CertWriter{f},
		}
		err := m.EnsureCerts(webhookConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.hasInvokedEnsureCerts).To(BeTrue())
	})

	It("should return the error from each CertWriter", func() {
		webhookConfig := &admissionregistration.MutatingWebhookConfiguration{}
		e := errors.New("this is an error")
		f := &fakeCertWriter{err: e}
		m := &MultiCertWriter{
			CertWriters: []CertWriter{f},
		}
		err := m.EnsureCerts(webhookConfig)
		Expect(err).To(MatchError(e))
		Expect(f.hasInvokedEnsureCerts).To(BeTrue())
	})
})
