package integration

import (
	"bytes"
	"os/exec"

	"github.com/kubernetes-sig-testing/frameworks/integration/internal"
)

// KubeCtl is a wrapper around the kubectl binary.
type KubeCtl struct {
	// Path where the kubectl binary can be found.
	//
	// If this is left empty, we will attempt to locate a binary, by checking for
	// the TEST_ASSET_KUBECTL environment variable, and the default test assets
	// directory. See the "Binaries" section above (in doc.go) for details.
	Path string

	// Opts can be used to configure additional flags which will be used each
	// time the wrapped binary is called.
	//
	// For example, you might want to use this to set the URL of the APIServer to
	// connect to.
	Opts []string

	// Stdout & Stderr capture and store both Stdout & Stderr of the binary.
	Stdout []byte
	Stderr []byte
}

// Run executes the wrapped binary with some preconfigured options and the
// arguments given to this method.
func (k *KubeCtl) Run(args ...string) error {
	if k.Path == "" {
		k.Path = internal.BinPathFinder("kubectl")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	allArgs := append(k.Opts, args...)

	cmd := exec.Command(k.Path, allArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	k.Stdout = stdout.Bytes()
	k.Stderr = stderr.Bytes()

	return err
}
