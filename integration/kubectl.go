package integration

import (
	"bytes"
	"os/exec"

	"github.com/kubernetes-sig-testing/frameworks/integration/internal"
)

// KubeCtl is a wrapper around the kubectl binary.
type KubeCtl struct {
	// Path where the kubectl binary can be found. If left empty, the
	// BinPathFinder will be used to determine the path to the binary.
	Path string

	// Opts can be used to configure certain option which should be used each
	// time the wrapped binary is called.
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
