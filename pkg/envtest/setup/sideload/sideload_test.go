package sideload_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/sideload"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/testhelpers"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

var (
	testLog logr.Logger
	ctx     context.Context
)

func TestEnv(t *testing.T) {
	testLog = testhelpers.GetLogger()
	ctx = logr.NewContext(context.Background(), testLog)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Sideload Suite")
}

var _ = Describe("Sideload", func() {
	var (
		prefix = "a-test-package"
		input  io.Reader
	)
	BeforeEach(func() {
		contents, err := testhelpers.ContentsFor(prefix)
		Expect(err).NotTo(HaveOccurred())

		input = bytes.NewReader(contents)
	})

	It("should fail if a non-concrete version is given", func() {
		err := sideload.Sideload(ctx, versions.LatestVersion)
		Expect(err).To(HaveOccurred())
	})

	It("should fail if a non-concrete platform is given", func() {
		err := sideload.Sideload(ctx, versions.Spec{Selector: &versions.Concrete{Major: 1, Minor: 2, Patch: 3}}, sideload.WithPlatform("*", "*"))
		Expect(err).To(HaveOccurred())
	})

	It("should load the given tarball into our store as the given version", func() {
		v := &versions.Concrete{Major: 1, Minor: 2, Patch: 3}
		store := testhelpers.NewMockStore()
		Expect(sideload.Sideload(
			ctx,
			versions.Spec{Selector: v},
			sideload.WithInput(input),
			sideload.WithEnvOptions(env.WithStore(store)),
		)).To(Succeed())

		baseName := versions.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}.BaseName(*v)
		expectedPath := filepath.Join("k8s", baseName, prefix)

		outFile, err := store.Root.Open(expectedPath)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(outFile.Close)
		contents, err := io.ReadAll(outFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(HavePrefix(prefix))
	})
})
