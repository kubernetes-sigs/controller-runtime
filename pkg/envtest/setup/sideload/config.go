package sideload

import (
	"io"
	"os"
	"runtime"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/env"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/versions"
)

type config struct {
	envOpts  []env.Option
	input    io.Reader
	platform versions.Platform
}

// Option is a functional option for configuring the sideload process.
type Option func(*config)

// WithEnvOptions configures the environment options for sideloading.
func WithEnvOptions(options ...env.Option) Option {
	return func(cfg *config) {
		cfg.envOpts = append(cfg.envOpts, options...)
	}
}

// WithInput configures the source to read the binary package from
func WithInput(input io.Reader) Option {
	return func(cfg *config) {
		cfg.input = input
	}
}

// WithPlatform sets the target OS and architecture for the sideload.
func WithPlatform(os string, arch string) Option {
	return func(cfg *config) {
		cfg.platform = versions.Platform{
			OS:   os,
			Arch: arch,
		}
	}
}

func configure(options ...Option) config {
	cfg := config{
		input: os.Stdin,
		platform: versions.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
	}

	for _, option := range options {
		option(&cfg)
	}

	return cfg
}
