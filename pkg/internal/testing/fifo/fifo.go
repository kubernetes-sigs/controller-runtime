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

// Package fifo contains helpers for writing to named pipes (FIFOs) in tests.
package fifo

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Create creates a FIFO at each of paths.
func Create(paths ...string) error {
	for _, path := range paths {
		if err := unix.Mkfifo(path, 0o600); err != nil {
			return fmt.Errorf("failed to create FIFO %s: %w", path, err)
		}
	}
	return nil
}

// WriteAndRemove opens the FIFO at path for writing, removes path, then writes
// data to the already-open descriptor and closes it. A reader that opened path
// before it was removed still receives data, but path no longer exists afterwards.
//
// Opening a FIFO for writing blocks until a reader opens it, so WriteAndRemove
// hangs waiting for a reader. It gives up and returns an error after 5 seconds,
// but the goroutine opening the FIFO stays blocked until a reader shows up.
func WriteAndRemove(path string, data []byte) (err error) {
	opened := make(chan *os.File, 1)
	openErr := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			openErr <- err
			return
		}
		opened <- f
	}()

	var f *os.File
	select {
	case f = <-opened:
	case err := <-openErr:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for a reader on %s", path)
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	if err := os.Remove(path); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}
