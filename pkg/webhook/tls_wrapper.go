/*
Copyright 2019 The Kubernetes Authors.

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
	"crypto/tls"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

type wrappedTLS struct {
	sync.Mutex
	certificate *tls.Certificate
	log         logr.Logger
}

func (w *wrappedTLS) setLogger(logger logr.Logger) error {
	w.log = logger
	return nil
}

func (w *wrappedTLS) getCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	w.Lock()
	defer w.Unlock()

	return w.certificate, nil
}

func (w *wrappedTLS) loadCertificate(cert, key string) error {
	w.Lock()
	defer w.Unlock()

	certKey, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return err
	}

	w.certificate = &certKey
	return nil
}

func (w *wrappedTLS) autoloadTLS(cert, key string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	done := make(chan error)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					w.log.Info("reload modified TLS:", event.Name)
					err := w.loadCertificate(cert, key)
					if err != nil {
						done <- err
					}
				}
			case err := <-watcher.Errors:
				done <- err
			}
		}
	}()

	err = watcher.Add(cert)
	if err != nil {
		done <- err
	}

	err = watcher.Add(key)
	if err != nil {
		done <- err
	}
	return <-done
}
