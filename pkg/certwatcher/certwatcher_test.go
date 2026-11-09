/*
Copyright 2021 The Kubernetes Authors.

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

package certwatcher_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher/metrics"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("CertWatcher", func() {
	var _ = Describe("certwatcher New", func() {
		It("should errors without cert/key", func() {
			_, err := certwatcher.New("", "")
			Expect(err).To(HaveOccurred())
		})
	})

	var _ = Describe("certwatcher Start", func() {
		var (
			ctxCancel    context.CancelFunc
			watcher      *certwatcher.CertWatcher
			startWatcher func(interval time.Duration) (done <-chan struct{})
		)

		BeforeEach(func() {
			var ctx context.Context
			ctx, ctxCancel = context.WithCancel(context.Background()) //nolint:forbidigo // the watcher outlives the BeforeEach

			err := writeCerts(certPath, keyPath, "127.0.0.1")
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				for _, file := range []string{certPath, keyPath} {
					_, err := os.ReadFile(file)
					if err != nil {
						return err
					}
					continue
				}

				return nil
			}).Should(Succeed())

			watcher, err = certwatcher.New(certPath, keyPath)
			Expect(err).ToNot(HaveOccurred())

			startWatcher = func(interval time.Duration) (done <-chan struct{}) {
				doneCh := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					defer close(doneCh)
					Expect(watcher.WithWatchInterval(interval).Start(ctx)).To(Succeed())
				}()
				// wait till we read first cert
				Eventually(func() error {
					err := watcher.ReadCertificate()
					return err
				}).Should(Succeed())
				return doneCh
			}
		})

		It("should not require LeaderElection", func() {
			leaderElectionRunnable, isLeaderElectionRunnable := any(watcher).(manager.LeaderElectionRunnable)
			Expect(isLeaderElectionRunnable).To(BeTrue())
			Expect(leaderElectionRunnable.NeedLeaderElection()).To(BeFalse())
		})

		It("should read the initial cert/key", func() {
			// This test verifies the initial read succeeded. So interval doesn't matter.
			doneCh := startWatcher(10 * time.Second)

			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
		})

		It("should reload currentCert when changed", func() {
			// This test verifies fsnotify detects the cert change. So interval doesn't matter.
			doneCh := startWatcher(10 * time.Second)
			called := atomic.Int64{}
			watcher.RegisterCallback(func(crt tls.Certificate) {
				called.Add(1)
				Expect(crt.Certificate).ToNot(BeEmpty())
			})

			firstcert, _ := watcher.GetCertificate(nil)

			err := writeCerts(certPath, keyPath, "192.168.0.1")
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				secondcert, _ := watcher.GetCertificate(nil)
				first := firstcert.PrivateKey.(*rsa.PrivateKey)
				return first.Equal(secondcert.PrivateKey) || firstcert.Leaf.SerialNumber == secondcert.Leaf.SerialNumber
			}).ShouldNot(BeTrue())

			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
			Expect(called.Load()).To(BeNumerically(">=", 1))
		})

		It("should reload currentCert when changed with rename", func() {
			// This test verifies fsnotify detects the cert change. So interval doesn't matter.
			doneCh := startWatcher(10 * time.Second)
			called := atomic.Int64{}
			watcher.RegisterCallback(func(crt tls.Certificate) {
				called.Add(1)
				Expect(crt.Certificate).ToNot(BeEmpty())
			})

			firstcert, _ := watcher.GetCertificate(nil)

			err := writeCerts(certPath+".new", keyPath+".new", "192.168.0.2")
			Expect(err).ToNot(HaveOccurred())

			Expect(os.Link(certPath, certPath+".old")).To(Succeed())
			Expect(os.Rename(certPath+".new", certPath)).To(Succeed())

			Expect(os.Link(keyPath, keyPath+".old")).To(Succeed())
			Expect(os.Rename(keyPath+".new", keyPath)).To(Succeed())

			Eventually(func() bool {
				secondcert, _ := watcher.GetCertificate(nil)
				first := firstcert.PrivateKey.(*rsa.PrivateKey)
				return first.Equal(secondcert.PrivateKey) || firstcert.Leaf.SerialNumber == secondcert.Leaf.SerialNumber
			}).ShouldNot(BeTrue())

			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
			Expect(called.Load()).To(BeNumerically(">=", 1))
		})

		It("should reload currentCert after move out", func() {
			// This test verifies poll works, so we'll use 1s as interval (fsnotify doesn't detect this change).
			doneCh := startWatcher(1 * time.Second)
			called := atomic.Int64{}
			watcher.RegisterCallback(func(crt tls.Certificate) {
				called.Add(1)
				Expect(crt.Certificate).ToNot(BeEmpty())
			})

			firstcert, _ := watcher.GetCertificate(nil)

			Expect(os.Rename(certPath, certPath+".old")).To(Succeed())
			Expect(os.Rename(keyPath, keyPath+".old")).To(Succeed())

			err := writeCerts(certPath, keyPath, "192.168.0.3")
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				secondcert, _ := watcher.GetCertificate(nil)
				first := firstcert.PrivateKey.(*rsa.PrivateKey)
				return first.Equal(secondcert.PrivateKey) || firstcert.Leaf.SerialNumber == secondcert.Leaf.SerialNumber
			}, "10s", "1s").ShouldNot(BeTrue())

			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
			Expect(called.Load()).To(BeNumerically(">=", 1))
		})

		It("should keep running when reading fails but the current certificate is still valid", func() {
			doneCh := startWatcher(100 * time.Millisecond)
			waitForPoll(testutil.ToFloat64(metrics.ReadCertificateTotal) + 1)

			Expect(os.Remove(keyPath)).To(Succeed())
			Consistently(doneCh, "1s").ShouldNot(BeClosed())

			ctxCancel()
			Eventually(doneCh, "4s").Should(BeClosed())
		})

		It("should return an error after three failed reads in a row once the current certificate has expired", func() {
			// X509KeyPair does not check expiry, so an already-expired certificate loads fine.
			Expect(writeCertsWithExpiry(certPath, keyPath, "127.0.0.1", time.Now().Add(-1*time.Minute))).To(Succeed())
			Expect(watcher.ReadCertificate()).To(Succeed())

			errCh := make(chan error, 1)
			go func() {
				defer GinkgoRecover()
				errCh <- watcher.WithWatchInterval(500 * time.Millisecond).Start(context.Background()) //nolint:forbidigo // Start must return on its own
			}()
			waitForPoll(testutil.ToFloat64(metrics.ReadCertificateTotal) + 1)

			errorsBefore := testutil.ToFloat64(metrics.ReadCertificateErrors)
			Expect(os.Remove(keyPath)).To(Succeed())

			// The first failed reads are tolerated.
			Consistently(errCh, "700ms").ShouldNot(Receive())

			Eventually(errCh, "5s").Should(Receive(MatchError(ContainSubstring("current one has expired"))))
			Expect(testutil.ToFloat64(metrics.ReadCertificateErrors)).To(BeNumerically(">=", errorsBefore+3))
		})

		Context("prometheus metric read_certificate_total", func() {
			var readCertificateTotalBefore float64
			var readCertificateErrorsBefore float64

			BeforeEach(func() {
				readCertificateTotalBefore = testutil.ToFloat64(metrics.ReadCertificateTotal)
				readCertificateErrorsBefore = testutil.ToFloat64(metrics.ReadCertificateErrors)
			})

			It("should get updated on successful certificate read", func() {
				Expect(watcher.ReadCertificate()).To(Succeed())

				Expect(testutil.ToFloat64(metrics.ReadCertificateTotal)).To(Equal(readCertificateTotalBefore + 1))
			})

			It("should get updated on read certificate errors", func() {
				Expect(os.Remove(keyPath)).To(Succeed())

				Expect(watcher.ReadCertificate()).To(HaveOccurred())

				Expect(testutil.ToFloat64(metrics.ReadCertificateTotal)).To(Equal(readCertificateTotalBefore + 1))
				Expect(testutil.ToFloat64(metrics.ReadCertificateErrors)).To(Equal(readCertificateErrorsBefore + 1))
			})
		})
	})
})

// waitForPoll waits until the watcher has polled the certificate, which
// happens only after its file watches are registered.
func waitForPoll(minReads float64) {
	GinkgoHelper()
	Eventually(func() float64 {
		return testutil.ToFloat64(metrics.ReadCertificateTotal)
	}, "15s").Should(BeNumerically(">=", minReads))
}

func writeCerts(certPath, keyPath, ip string) error {
	return writeCertsWithExpiry(certPath, keyPath, ip, time.Now().Add(1*time.Hour))
}

func writeCertsWithExpiry(certPath, keyPath, ip string, notAfter time.Time) error {
	var priv any
	var err error
	priv, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	keyUsage := x509.KeyUsageDigitalSignature
	if _, isRSA := priv.(*rsa.PrivateKey); isRSA {
		keyUsage |= x509.KeyUsageKeyEncipherment
	}

	notBefore := time.Now()

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Kubernetes"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              keyUsage,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	template.IPAddresses = append(template.IPAddresses, net.ParseIP(ip))

	privkey := priv.(*rsa.PrivateKey)

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privkey.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}
	if err := certOut.Close(); err != nil {
		return err
	}

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return err
	}
	return keyOut.Close()
}
