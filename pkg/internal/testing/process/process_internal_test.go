/*
Copyright 2026 The Kubernetes Authors.

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

package process

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStopReturnsTimeoutWhenKillAfterTimeoutFails(t *testing.T) {
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find current process: %v", err)
	}

	originalSignalProcess := signalProcess
	t.Cleanup(func() {
		signalProcess = originalSignalProcess
	})

	signalCount := 0
	signalProcess = func(_ *os.Process, sig syscall.Signal) error {
		signalCount++
		if sig == syscall.SIGKILL {
			return errors.New("kill failed")
		}
		return nil
	}

	ps := &State{
		Cmd:         &exec.Cmd{Process: process},
		Path:        "/path/to/test-process",
		StopTimeout: time.Nanosecond,
		waitDone:    make(chan struct{}),
	}

	err = ps.Stop()
	if err == nil {
		t.Fatal("expected Stop to return an error")
	}
	if !strings.Contains(err.Error(), "timeout waiting for process test-process to stop") {
		t.Fatalf("expected timeout error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "unable to kill process /path/to/test-process: kill failed") {
		t.Fatalf("expected kill error to be included, got %q", err.Error())
	}
	if signalCount != 2 {
		t.Fatalf("expected SIGTERM and SIGKILL to be sent, got %d signals", signalCount)
	}
}
