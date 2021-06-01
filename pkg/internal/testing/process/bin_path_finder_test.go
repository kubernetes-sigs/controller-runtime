package process

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("BinPathFinder", func() {
	var prevAssetPath string
	BeforeEach(func() {
		prevAssetPath = os.Getenv(EnvAssetsPath)
		Expect(os.Unsetenv(EnvAssetsPath)).To(Succeed())
		Expect(os.Unsetenv(EnvAssetOverridePrefix + "_SOME_FAKE"))
		Expect(os.Unsetenv(EnvAssetOverridePrefix + "OTHERFAKE"))
	})
	AfterEach(func() {
		if prevAssetPath != "" {
			Expect(os.Setenv(EnvAssetsPath, prevAssetPath))
		}
	})
	Context("when individual overrides are present", func() {
		BeforeEach(func() {
			Expect(os.Setenv(EnvAssetOverridePrefix+"OTHERFAKE", "/other/path")).To(Succeed())
			Expect(os.Setenv(EnvAssetOverridePrefix+"_SOME_FAKE", "/some/path")).To(Succeed())
			// set the global path to make sure we don't prefer it
			Expect(os.Setenv(EnvAssetsPath, "/global/path")).To(Succeed())
		})

		It("should prefer individual overrides, using them unmodified", func() {
			Expect(BinPathFinder("otherfake", "/hardcoded/path")).To(Equal("/other/path"))
		})

		It("should convert lowercase to uppercase, remove leading numbers, and replace punctuation with underscores when resolving the env var name", func() {
			Expect(BinPathFinder("123.some-fake", "/hardcoded/path")).To(Equal("/some/path"))
		})
	})

	Context("when individual overrides are missing but the global override is present", func() {
		BeforeEach(func() {
			Expect(os.Setenv(EnvAssetsPath, "/global/path")).To(Succeed())
		})
		It("should prefer the global override, appending the name to that path", func() {
			Expect(BinPathFinder("some-fake", "/hardcoded/path")).To(Equal("/global/path/some-fake"))
		})
	})

	Context("when an asset directory is given and no overrides are present", func() {
		It("should use the asset directory, appending the name to that path", func() {
			Expect(BinPathFinder("some-fake", "/hardcoded/path")).To(Equal("/hardcoded/path/some-fake"))
		})
	})

	Context("when no path configuration is given", func() {
		It("should just use the default path", func() {
			Expect(BinPathFinder("some-fake", "")).To(Equal("/usr/local/kubebuilder/bin/some-fake"))
		})
	})
})
