package list

import (
	"runtime"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

type config struct {
	platform  versions.Platform
	localOnly bool
	envOpts   []env.Option
}

// Option is a functional option for configuring the list workflow
type Option func(*config)

// WithEnvOptions provides options for the env.Env used by the workflow
func WithEnvOptions(opts ...env.Option) Option {
	return func(c *config) { c.envOpts = append(c.envOpts, opts...) }
}

// WithPlatform sets the target OS and architecture for the download.
func WithPlatform(os string, arch string) Option {
	return func(c *config) { c.platform = versions.Platform{OS: os, Arch: arch} }
}

// NoDownload ensures only local versions are considered
func NoDownload(noDownload bool) Option { return func(c *config) { c.localOnly = noDownload } }

func configure(options ...Option) *config {
	cfg := &config{}

	for _, opt := range options {
		opt(cfg)
	}

	if cfg.platform == (versions.Platform{}) {
		cfg.platform = versions.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		}
	}
	return cfg
}
