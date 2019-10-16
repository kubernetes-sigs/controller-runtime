package setup

import (
	"fmt"
	"net/url"
	"net"

	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/internal"
)

// TODO(directxman12): is there a way to just keep TinyCA internal?

// NewTinyCA creates a new a tiny CA utilitiy for provisioning serving certs and client certs FOR TESTING ONLY.
// Don't use this for anything else!
var NewTinyCA = internal.NewTinyCA

// UserProvisioner knows how to make users that can be used with the Kubernetes API server,
// and configure the API server to make use of its users.
type UserProvisioner = internal.UserProvisioner

// ControlPlane is a struct that knows how to start your test control plane.
//
// Right now, that means Etcd and your APIServer. This is likely to increase in
// future.
type ControlPlane struct {
	APIServer *APIServer
	Etcd      *Etcd

	// UserProvisioners enables running with authentication to the API server.
	// They are responsible for configuring the API server appropriately and
	// provisioning new users.  They may use certificates, token auth, etc.
	UserProvisioners []UserProvisioner
}

// Start will start your control plane processes. To stop them, call Stop().
func (f *ControlPlane) Start() error {
	if f.Etcd == nil {
		f.Etcd = &Etcd{}
	}
	if err := f.Etcd.Start(); err != nil {
		return err
	}

	if f.APIServer == nil {
		f.APIServer = &APIServer{}
	}
	f.APIServer.EtcdURL = f.Etcd.URL
	if len(f.UserProvisioners) > 0 {
		for _, prov := range f.UserProvisioners {
			flags, err := prov.Start()
			if err != nil {
				return err
			}
			f.APIServer.Args = append(f.APIServer.Args, flags...)
		}
	}
	return f.APIServer.Start()
}

// Stop will stop your control plane processes, and clean up their data.
func (f *ControlPlane) Stop() error {
	if f.APIServer != nil {
		if err := f.APIServer.Stop(); err != nil {
			return err
		}
	}
	if f.Etcd != nil {
		if err := f.Etcd.Stop(); err != nil {
			return err
		}
	}
	if len(f.UserProvisioners) > 0 {
		for _, prov := range f.UserProvisioners {
			if err := prov.Stop(); err != nil {
				return err
			}
		}
	}
	return nil
}

// APIURL returns the URL you should connect to to talk to your API.
func (f *ControlPlane) APIURL() *url.URL {
	return f.APIServer.URL
}

// SecureURL returns the URL you can use to communicae with the API server over the secure port.
// TODO: this probably isn't quite correct if the user configured a URL, but
// user configuration is dubious anyway.
func (f *ControlPlane) SecureURL() *url.URL {
	baseURL := *f.APIServer.URL
	host, _, err := net.SplitHostPort(baseURL.Host)
	if err != nil {
		host = baseURL.Host
	}
	baseURL.Host = net.JoinHostPort(host, fmt.Sprintf("%d", f.APIServer.SecurePort))
	return &baseURL
}

// KubeCtl returns a pre-configured KubeCtl, ready to connect to this
// ControlPlane.
func (f *ControlPlane) KubeCtl() *KubeCtl {
	k := &KubeCtl{}
	k.Opts = append(k.Opts, fmt.Sprintf("--server=%s", f.APIURL()))
	return k
}
