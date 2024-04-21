package env

import (
	"io/fs"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/remote"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/store"
)

// KubebuilderAssetsEnvVar is the environment variable that can be used to override the default local storage location
const KubebuilderAssetsEnvVar = "KUBEBUILDER_ASSETS"

// Env encapsulates the environment dependencies.
type Env struct {
	*store.Store
	remote.Client
	fs.FS
}

// Option is a functional option for configuring an environment
type Option func(*Env)

// WithStoreAt sets the path to the store directory.
func WithStoreAt(dir string) Option {
	return func(c *Env) { c.Store = store.NewAt(dir) }
}

// WithStore allows injecting a envured store.
func WithStore(store *store.Store) Option {
	return func(c *Env) { c.Store = store }
}

// WithClient allows injecting a envured remote client.
func WithClient(client remote.Client) Option { return func(c *Env) { c.Client = client } }

// WithFS allows injecting a configured fs.FS, e.g. for mocking.
// TODO: fix this so it's actually used!
func WithFS(fs fs.FS) Option {
	return func(c *Env) {
		c.FS = fs
	}
}

// New returns a new environment, configured with the provided options.
//
// If no options are provided, it will be created with a production store.Store and remote.Client
// and an OS file system.
func New(options ...Option) (*Env, error) {
	env := &Env{
		// this is the minimal configuration that won't panic
		Client: &remote.GCSClient{ //nolint:staticcheck
			Bucket: remote.DefaultBucket, //nolint:staticcheck
			Server: remote.DefaultServer, //nolint:staticcheck
			Log:    logr.Discard(),
		},
	}

	for _, option := range options {
		option(env)
	}

	if env.Store == nil {
		dir, err := store.DefaultStoreDir()
		if err != nil {
			return nil, err
		}
		env.Store = store.NewAt(dir)
	}

	return env, nil
}
