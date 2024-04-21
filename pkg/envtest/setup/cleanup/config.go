package cleanup

import (
	"runtime"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

type config struct {
	envOpts  []env.Option
	platform versions.Platform
}

// Option is a functional option for configuring the cleanup process.
type Option func(*config)

// WithEnvOptions adds options to the environment setup.
func WithEnvOptions(opts ...env.Option) Option {
	return func(cfg *config) {
		cfg.envOpts = append(cfg.envOpts, opts...)
	}
}

// WithPlatform sets the platform to use for cleanup.
func WithPlatform(os string, arch string) Option {
	return func(cfg *config) {
		cfg.platform = versions.Platform{OS: os, Arch: arch}
	}
}

func configure(options ...Option) *config {
	cfg := &config{
		platform: versions.Platform{
			Arch: runtime.GOARCH,
			OS:   runtime.GOOS,
		},
	}

	for _, opt := range options {
		opt(cfg)
	}

	return cfg
}
