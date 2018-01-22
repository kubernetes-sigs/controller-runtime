package integration_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/kubernetes-sig-testing/frameworks/integration"
)

var _ = Describe("Kubectl", func() {
	It("runs kubectl", func() {
		k := &KubeCtl{Path: "bash"}
		args := []string{"-c", "echo 'something'"}

		Expect(k.Run(args...)).To(Succeed())
		Expect(k.Stdout).To(ContainSubstring("something"))
		Expect(k.Stderr).To(BeEmpty())
	})

	Context("when the command returns a non-zero exit code", func() {
		It("returns an error", func() {
			k := &KubeCtl{Path: "bash"}
			args := []string{
				"-c", "echo 'this is StdErr' >&2; echo 'but this is StdOut' >&1; exit 66",
			}

			err := k.Run(args...)

			Expect(err).To(MatchError(ContainSubstring("exit status 66")))

			Expect(k.Stdout).To(ContainSubstring("but this is StdOut"))
			Expect(k.Stderr).To(ContainSubstring("this is StdErr"))
		})
	})
})
