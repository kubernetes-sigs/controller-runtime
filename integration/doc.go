/*

Package integration implements an integration testing framework for kubernetes.

It provides components for standing up a kubernetes API, against which you can test a
kubernetes client, or other kubernetes components. The lifecycle of the components
needed to provide this API is managed by this framework.

Quickstart

If you want to test a kubernetes client against the latest kubernetes APIServer
and Etcd, you can use `./scripts/download-binaries.sh` to download APIServer
and Etcd binaries for your platform. Then add something like the following to
your tests:

	cp := &integration.ControlPlane{}
	cp.Start()
	kubeCtl := cp.KubeCtl()
	stdout, stderr, err := kubeCtl.Run("get", "pods")
	// You can check on err, stdout & stderr and build up
	// your tests
	cp.Stop()

Components

Currently the framework provides the following components:

ControlPlane: The ControlPlane wraps Etcd & APIServer (see below) and wires
them together correctly. A ControlPlane can be stopped & started and can
provide the URL to connect to the API. The ControlPlane can also be asked for a
KubeCtl which is already correctly configured for this ControlPlane. The
ControlPlane is a good entry point for default setups.

Etcd: Manages an Etcd binary, which can be started, stopped and connected to.
By default Etcd will listen on a random port for http connections and will
create a temporary directory for its data. To configure it differently, see the
Etcd type documentation below.

APIServer: Manages an Kube-APIServer binary, which can be started, stopped and
connected to. By default APIServer will listen on a random port for http
connections and will create a temporary directory to store the (auto-generated)
certificates.  To configure it differently, see the APIServer type
documentation below.

KubeCtl: Wraps around a `kubectl` binary and can `Run(...)` arbitrary commands
against a kubernetes control plane.

Binaries

Both Etcd and APIServer use the same mechanism to determine which binaries to
use when they get started.

1. If the component is configured with a `Path` the framework tries to run that
binary.
For example:

	myEtcd := &Etcd{
		Path: "/some/other/etcd",
	}
	cp := &integration.ControlPlane{
		Etcd: myEtcd,
	}
	cp.Start()

2. If the Path field on APIServer or Etcd is left unset and an environment
variable named `TEST_ASSET_KUBE_APISERVER` or `TEST_ASSET_ETCD` is set, its
value is used as a path to the binary for the APIServer or Etcd.

3. If neither the `Path` field, nor the environment variable is set, the
framework tries to use the binaries `kube-apiserver` or `etcd` in the directory
`${FRAMEWORK_DIR}/assets/bin/`.

For convenience this framework ships with
`${FRAMEWORK_DIR}/scripts/download-binaries.sh` which can be used to download
pre-compiled versions of the needed binaries and place them in the default
location (`${FRAMEWORK_DIR}/assets/bin/`).

*/
package integration
