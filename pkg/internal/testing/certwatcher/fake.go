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

// Package certwatcher contains a fake certificate watcher for tests.
package certwatcher

import (
	"context"
	"crypto/tls"
)

// Fake is a fake certificate watcher with the same methods as
// *certwatcher.CertWatcher that the servers use.
type Fake struct {
	// StartErr is returned by Start.
	StartErr error
}

// GetCertificate returns an empty certificate.
func (f *Fake) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return &tls.Certificate{}, nil
}

// Start returns StartErr immediately.
func (f *Fake) Start(context.Context) error {
	return f.StartErr
}
