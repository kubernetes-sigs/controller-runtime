package internal

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

type ProcessState struct {
	DefaultedProcessInput
	Session *gexec.Session
	// Healthcheck Endpoint. If we get http.StatusOK from this endpoint, we
	// assume the process is ready to operate. E.g. "/healthz". If this is set,
	// we ignore StartMessage.
	HealthCheckEndpoint string
	// StartMessage is the message to wait for on stderr. If we recieve this
	// message, we assume the process is ready to operate. Ignored if
	// HealthCheckEndpoint is specified.
	//
	// The usage of StartMessage is discouraged, favour HealthCheckEndpoint
	// instead!
	//
	// Deprecated: Use HealthCheckEndpoint in favour of StartMessage
	StartMessage string
	Args         []string
}

type DefaultedProcessInput struct {
	URL              url.URL
	Dir              string
	DirNeedsCleaning bool
	Path             string
	StopTimeout      time.Duration
	StartTimeout     time.Duration
}

func DoDefaulting(
	name string,
	listenUrl *url.URL,
	dir string,
	path string,
	startTimeout time.Duration,
	stopTimeout time.Duration,
) (DefaultedProcessInput, error) {
	defaults := DefaultedProcessInput{
		Dir:          dir,
		Path:         path,
		StartTimeout: startTimeout,
		StopTimeout:  stopTimeout,
	}

	if listenUrl == nil {
		am := &AddressManager{}
		port, host, err := am.Initialize()
		if err != nil {
			return DefaultedProcessInput{}, err
		}
		defaults.URL = url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", host, port),
		}
	} else {
		defaults.URL = *listenUrl
	}

	if dir == "" {
		newDir, err := ioutil.TempDir("", "k8s_test_framework_")
		if err != nil {
			return DefaultedProcessInput{}, err
		}
		defaults.Dir = newDir
		defaults.DirNeedsCleaning = true
	}

	if path == "" {
		if name == "" {
			return DefaultedProcessInput{}, fmt.Errorf("must have at least one of name or path")
		}
		defaults.Path = BinPathFinder(name)
	}

	if startTimeout == 0 {
		defaults.StartTimeout = 20 * time.Second
	}

	if stopTimeout == 0 {
		defaults.StopTimeout = 20 * time.Second
	}

	return defaults, nil
}

func (ps *ProcessState) Start(stdout, stderr io.Writer) (err error) {
	command := exec.Command(ps.Path, ps.Args...)

	ready := make(chan bool)
	timedOut, timeOutPoller := teeTimer(time.After(ps.StartTimeout))

	if ps.HealthCheckEndpoint != "" {
		healthCheckURL := ps.URL
		healthCheckURL.Path = ps.HealthCheckEndpoint
		go pollURLUntilOK(healthCheckURL, ready, timeOutPoller)
	} else {
		startDetectStream := gbytes.NewBuffer()
		ready = startDetectStream.Detect(ps.StartMessage)
		stderr = safeMultiWriter(stderr, startDetectStream)
	}

	ps.Session, err = gexec.Start(command, stdout, stderr)
	if err != nil {
		return err
	}

	select {
	case <-ready:
		return nil
	case <-timedOut:
		if ps.Session != nil {
			ps.Session.Terminate()
		}
		return fmt.Errorf("timeout waiting for process %s to start", path.Base(ps.Path))
	}
}

func teeTimer(src <-chan time.Time) (chan time.Time, chan time.Time) {
	destLeft := make(chan time.Time)
	destRight := make(chan time.Time)

	go func() {
		for {
			t, more := <-src
			destLeft <- t
			destRight <- t
			if !more {
				close(destLeft)
				close(destRight)
			}
		}
	}()

	return destLeft, destRight
}

func safeMultiWriter(writers ...io.Writer) io.Writer {
	safeWriters := []io.Writer{}
	for _, w := range writers {
		if w != nil {
			safeWriters = append(safeWriters, w)
		}
	}
	return io.MultiWriter(safeWriters...)
}

func pollURLUntilOK(url url.URL, ready chan bool, timedOut <-chan time.Time) {
	checker := func() bool {
		res, err := http.Get(url.String())
		if err == nil && res.StatusCode == http.StatusOK {
			ready <- true
			return true
		}
		return false
	}

	if isReady := checker(); isReady {
		return
	}

	for {
		select {
		case <-time.Tick(time.Millisecond * 100):
			if isReady := checker(); isReady {
				return
			}
		case <-timedOut:
			return
		}
	}
}

func (ps *ProcessState) Stop() error {
	if ps.Session == nil {
		return nil
	}

	// gexec's Session methods (Signal, Kill, ...) do not check if the Process is
	// nil, so we are doing this here for now.
	// This should probably be fixed in gexec.
	if ps.Session.Command.Process == nil {
		return nil
	}

	detectedStop := ps.Session.Terminate().Exited
	timedOut := time.After(ps.StopTimeout)

	select {
	case <-detectedStop:
		break
	case <-timedOut:
		return fmt.Errorf("timeout waiting for process %s to stop", path.Base(ps.Path))
	}

	if ps.DirNeedsCleaning {
		return os.RemoveAll(ps.Dir)
	}

	return nil
}
