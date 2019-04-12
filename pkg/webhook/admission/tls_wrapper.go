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

package admission

import (
	"crypto/tls"
	"sync"
)

type wrappedTls struct {
	sync.Mutex
	certificate *tls.Certificate
}

func (w *wrappedTls) getCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	w.Lock()
	defer w.Unlock()

	return w.certificate, nil
}

func (w *wrappedTls) loadCertificate(cert, key string) error {
	w.Lock()
	defer w.Unlock()

	certKey, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return err
	}

	w.certificate = &certKey
	return nil
}
