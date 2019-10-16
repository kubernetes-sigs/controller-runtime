package internal

import (
	"k8s.io/client-go/rest"
)

// UserProvisioner knows how to make users that can be used with the Kubernetes API server,
// and configure the API server to make use of its users.
type UserProvisioner interface {
	// RegisterUser creates a new user, populating the given REST configuration with appropriate auth details.
	RegisterUser(info User, cfg *rest.Config) error

	// Start starts this auth provider, and returns the flags to add to the API
	// server to launch it so that it respects this user provisioner.
	Start() (apiServerConfig []string, err error)

	// Stop stops this auth provider and cleans up any resources associated with it.
	Stop() error
}
