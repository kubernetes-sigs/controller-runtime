# examples/fleet

The `fleet` example is a basic implementation of a multi-cluster-capable controller. As a cluster provider it uses `kind`: Every local kind cluster with a `fleet-` name prefix will be picked up as dynamic cluster if multi-cluster support is enabled in `fleet`.

`fleet` can switch between multi-cluster mode and single-cluster mode via the `--enable-cluster-provider` flag (defaults to `true`) to demonstrate the seamless switch of using the same code base as a "normal" single-cluster controller and as a multi-cluster controller as easy as plugging in a (third-party) cluster provider. To run `fleet` against the current `KUBECONFIG` in single-cluster mode, just run:

```sh
$ USE_EXISTING_CLUSTER=true go run . --enable-cluster-provider=false
```
