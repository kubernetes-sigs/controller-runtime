package integration

import (
	"bytes"
	"os/exec"

	"github.com/kubernetes-sig-testing/frameworks/integration/internal"
)

type KubeCtl struct {
	Path string

	Stdout []byte
	Stderr []byte

	Opts []string
}

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
