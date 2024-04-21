package use

import (
	"cmp"
	"os"
	"runtime"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

type config struct {
	platform      versions.Platform
	assetPath     string
	noDownload    bool
	forceDownload bool
	verifySum     bool

	envOpts []env.Option
}

// Option is a functional option for configuring the use workflow
type Option func(*config)

// WithAssetsAt sets the path to the assets directory.
func WithAssetsAt(dir string) Option {
	return func(c *config) { c.assetPath = dir }
}

// WithAssetsFromEnv sets the path to the assets directory from the environment.
func WithAssetsFromEnv(useEnv bool) Option {
	return func(c *config) {
		if useEnv {
			c.assetPath = cmp.Or(os.Getenv(env.KubebuilderAssetsEnvVar), c.assetPath)
		}
	}
}

// ForceDownload forces the download of the specified version, even if it's already present.
func ForceDownload(force bool) Option { return func(c *config) { c.forceDownload = force } }

// NoDownload ensures only local versions are considered
func NoDownload(noDownload bool) Option { return func(c *config) { c.noDownload = noDownload } }

// WithPlatform sets the target OS and architecture for the download.
func WithPlatform(os string, arch string) Option {
	return func(c *config) { c.platform = versions.Platform{OS: os, Arch: arch} }
}

// WithEnvOptions provides options for the env.Env used by the workflow
func WithEnvOptions(opts ...env.Option) Option {
	return func(c *config) { c.envOpts = append(c.envOpts, opts...) }
}

// VerifySum turns on md5 verification of the downloaded package
func VerifySum(verify bool) Option { return func(c *config) { c.verifySum = verify } }

func configure(options ...Option) *config {
	cfg := &config{
		platform: versions.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
	}

	for _, opt := range options {
		opt(cfg)
	}

	return cfg
}
