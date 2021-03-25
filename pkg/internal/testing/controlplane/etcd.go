package controlplane

import (
	"io"
	"time"

	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/internal/testing/process"
)

// Etcd knows how to run an etcd server.
type Etcd struct {
	// URL is the address the Etcd should listen on for client connections.
	//
	// If this is not specified, we default to a random free port on localhost.
	URL *url.URL

	// Path is the path to the etcd binary.
	//
	// If this is left as the empty string, we will attempt to locate a binary,
	// by checking for the TEST_ASSET_ETCD environment variable, and the default
	// test assets directory. See the "Binaries" section above (in doc.go) for
	// details.
	Path string

	// Args is a list of arguments which will passed to the Etcd binary. Before
	// they are passed on, the`y will be evaluated as go-template strings. This
	// means you can use fields which are defined and exported on this Etcd
	// struct (e.g. "--data-dir={{ .Dir }}").
	// Those templates will be evaluated after the defaulting of the Etcd's
	// fields has already happened and just before the binary actually gets
	// started. Thus you have access to calculated fields like `URL` and others.
	//
	// If not specified, the minimal set of arguments to run the Etcd will be
	// used.
	Args []string

	// DataDir is a path to a directory in which etcd can store its state.
	//
	// If left unspecified, then the Start() method will create a fresh temporary
	// directory, and the Stop() method will clean it up.
	DataDir string

	// StartTimeout, StopTimeout specify the time the Etcd is allowed to
	// take when starting and stopping before an error is emitted.
	//
	// If not specified, these default to 20 seconds.
	StartTimeout time.Duration
	StopTimeout  time.Duration

	// Out, Err specify where Etcd should write its StdOut, StdErr to.
	//
	// If not specified, the output will be discarded.
	Out io.Writer
	Err io.Writer

	processState *process.ProcessState
}

// Start starts the etcd, waits for it to come up, and returns an error, if one
// occoured.
func (e *Etcd) Start() error {
	if e.processState == nil {
		if err := e.setProcessState(); err != nil {
			return err
		}
	}
	return e.processState.Start(e.Out, e.Err)
}

func (e *Etcd) setProcessState() error {
	var err error

	e.processState = &process.ProcessState{}

	e.processState.DefaultedProcessInput, err = process.DoDefaulting(
		"etcd",
		e.URL,
		e.DataDir,
		e.Path,
		e.StartTimeout,
		e.StopTimeout,
	)
	if err != nil {
		return err
	}

	e.processState.StartMessage = getEtcdStartMessage(e.processState.URL)

	e.URL = &e.processState.URL
	e.DataDir = e.processState.Dir
	e.Path = e.processState.Path
	e.StartTimeout = e.processState.StartTimeout
	e.StopTimeout = e.processState.StopTimeout

	args := e.Args
	if len(args) == 0 {
		args = EtcdDefaultArgs
	}

	e.processState.Args, err = process.RenderTemplates(args, e)
	return err
}

// Stop stops this process gracefully, waits for its termination, and cleans up
// the DataDir if necessary.
func (e *Etcd) Stop() error {
	return e.processState.Stop()
}

// EtcdDefaultArgs exposes the default args for Etcd so that you
// can use those to append your own additional arguments.
var EtcdDefaultArgs = []string{
	"--listen-peer-urls=http://localhost:0",
	"--advertise-client-urls={{ if .URL }}{{ .URL.String }}{{ end }}",
	"--listen-client-urls={{ if .URL }}{{ .URL.String }}{{ end }}",
	"--data-dir={{ .DataDir }}",
}

// isSecureScheme returns false when the schema is insecure.
func isSecureScheme(scheme string) bool {
	// https://github.com/coreos/etcd/blob/d9deeff49a080a88c982d328ad9d33f26d1ad7b6/pkg/transport/listener.go#L53
	if scheme == "https" || scheme == "unixs" {
		return true
	}
	return false
}

// getEtcdStartMessage returns an start message to inform if the client is or not insecure.
// It will return true when the URL informed has the scheme == "https" || scheme == "unixs"
func getEtcdStartMessage(listenURL url.URL) string {
	if isSecureScheme(listenURL.Scheme) {
		// https://github.com/coreos/etcd/blob/a7f1fbe00ec216fcb3a1919397a103b41dca8413/embed/serve.go#L167
		return "serving client requests on "
	}

	// https://github.com/coreos/etcd/blob/a7f1fbe00ec216fcb3a1919397a103b41dca8413/embed/serve.go#L124
	return "serving insecure client requests on "
}
