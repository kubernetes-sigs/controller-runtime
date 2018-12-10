## Walkthrough of creating a new operator

This walkthrough is for creating a new operator, to run the kubernetes
dashboard.

### Basics

Install kubebuilder from https://github.com/kubernetes-sigs/kubebuilder/releases if you have not yet already done so.

Create a new directory and use kubebuilder to create a basic operator for you:

```
mkdir dashboard-operator/
cd dashboard-operator/
kubebuilder init --domain sigs.k8s.io --license apache2 --owner "The Kubernetes Authors" --dep=true --pattern=addon
```

### Adding our first CRD

```
kubebuilder create api --group addons --version v1alpha1 --kind Dashboard --controller --resource --namespaced=true --pattern=addon
```

This creates API type definitions under `pkg/apis/addons/v1alpha1/`, and a basic
controller under `pkg/controller/dashboard/`

* Generate code: `go generate ./pkg/... ./cmd/...` (or `make generate`)

* You should now be able to `go run ./cmd/manager/main.go` (or `make run`),
  though it will exit with an error from being unable to find the dashboard CRD.

### Adding a manifest

The addon operator pattern is based on declarative manifests; the framework is
able to load the manifests and apply them.  Today we exec `kubectl apply`, but
when server-side-apply is available we'll use that.  We suggest that even
advanced operators should use a manifest for their core objects, but will be
able to manipulate the manifest before applying them (for example adding labels
or changing namespaces, and likely tweaking resources or flags as well.)

Some other advantages:

* Working with manifests lets us release a new dashboard version without needing
  a new operator version
* The declarative manifest makes it easier for users to understand what is
  changing in each version
* It should result in less / simpler code

For now, we embed the manifests into the image, but we'll be evolving this, for example sourcing manifests from a bundle or over https.

Create a manifest under `channels/packages/<packagename>/<version>/manifest.yaml`

```bash
mkdir -p channels/packages/dashboard/1.8.3/
wget -O channels/packages/dashboard/1.8.3/manifest.yaml https://raw.githubusercontent.com/kubernetes/dashboard/v1.8.3/src/deploy/recommended/kubernetes-dashboard.yaml
```

We have a notion of "channels", which is a stream of updates.  We'll have
settings to automatically update or prompt-for-update when the channel updates.
Currently if you don't specify a channel in your CRD, you get the version
currently in the stable channel.

We need to define the default stable channel, so create `channels/stable`:

```bash
cat > channels/stable <<EOF
manifests:
- version: 1.8.3
EOF
```

### Adding the framework into our types

We now want to plug the framework into our types, so that our CRs will look like
other addon operators.

We begin by editing the api type, we add some common fields.  The idea is that
CommonSpec and CommonStatus form a common contract that we expect all addons to
support (and we can hopefully evolve the contract with just a recompile!)

So in `pkg/apis/addons/v1alpha1/dashboard_types.go`:

* add an import for `addonv1alpha1 "sigs.k8s.io/controller-runtime/alpha/patterns/addon/pkg/apis/v1alpha1"`
* add a field `addon.CommonSpec` to the Spec object (and remove the placeholders)
* add a field `addon.CommonStatus` to the Status object (and remove the placeholders)

We'll also need to add some accessor functions (we could use reflection, but
this doesn't feel too onerous - ?):

```go
import addonv1alpha1 "sigs.k8s.io/controller-runtime/alpha/patterns/addon/pkg/apis/v1alpha1"

var _ addonv1alpha1.CommonObject = &Dashboard{}

func (c *Dashboard) ComponentName() string {
	return "dashboard"
}

func (c *Dashboard) CommonSpec() addonv1alpha1.CommonSpec {
	return c.Spec.CommonSpec
}

func (c *Dashboard) GetCommonStatus() addonv1alpha1.CommonStatus {
	return c.Status.CommonStatus
}

func (c *Dashboard) SetCommonStatus(s addonv1alpha1.CommonStatus) {
	c.Status.CommonStatus = s
}

```

### Using the framework in the controller

We replace the controller code `pkg/controller/dashboard/dashboard_controller.go`:

We are delegating most of the logic to `operators.StandardReconciler`

```go
/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dashboard

import (
	api "sigs.k8s.io/controller-runtime/alpha/patterns/addon/examples/dashboard-operator/pkg/apis/addons/v1alpha1"
	"sigs.k8s.io/controller-runtime/alpha/patterns/addon/pkg/status"
	"sigs.k8s.io/controller-runtime/alpha/patterns/declarative"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var _ reconcile.Reconciler = &ReconcileDashboard{}

// ReconcileDashboard reconciles a Dashboard object
type ReconcileDashboard struct {
	declarative.Reconciler
}

func Add(mgr manager.Manager) error {
	labels := map[string]string{
		"k8s-app": "kubernetes-dashboard",
	}

	r := &ReconcileDashboard{}

	r.Reconciler.Init(mgr, &api.Dashboard{}, "dashboard",
		declarative.WithObjectTransform(declarative.AddLabels(labels)),
		declarative.WithOwner(declarative.SourceAsOwner),
		declarative.WithLabels(declarative.SourceLabel),
		declarative.WithStatus(status.NewBasic(mgr.GetClient())),
	)

	c, err := controller.New("dashboard-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Dashboard
	err = c.Watch(&source.Kind{Type: &api.Dashboard{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to deployed objects
	_, err = declarative.WatchAll(mgr.GetConfig(), c, r, declarative.SourceLabel)
	if err != nil {
		return err
	}

	return nil
}

```

The important things to note here:

```go
	r.Reconciler.Init(mgr, &api.Dashboard{}, "dashboard", ...)
```

We bind the `api.Dashboard` type to the `dashboard` package in our `channels`
directory and pull in optional features of the declarative library.

Because api.Dashboard implements `addon.CommonObject` the
framework is then able to access CommonSpec and CommonStatus above, which
includes the version specifier.

We still need to specify which objects we watch in the `add` function, but we'll
likely be able to simplify this also in future.

### Misc

* Please add this boilerplate to the top of the main() function in cmd/manager/main.go:

```go
addon.Init()
```

### Testing it locally

We can register the Dashboard CRD and a Dashboard object, and then try running
the controller locally.

We need to generate and register the CRDs:

```bash
make manifests
kubectl apply -f config/crds/
```

Create a dashboard CR:

```bash
kubectl apply -n kube-system -f config/samples/addons_v1alpha1_dashboard.yaml
```

You should now be able to run the controller using:

`make run`

You should see your operator apply the manifest.  You can then control-C and you
should see the deployment etc that the operator has created.

e.g. `kubectl get pods -l k8s-app=kubernetes-dashboard`

### Running on-cluster

Previously we were running on your machine, using your kubernetes credentials.
For real-world operation, we want to run as a Pod on the cluster, likely launched as part
of a Deployment.  For that, we'll need a Docker image and some manifests.

We can use bazel to create a docker image and remove the autogenerated image.

```bazel
rm Dockerfile
cat > BUILD.bazel <<EOF
load(
  "@io_bazel_rules_docker//container:container.bzl",
  "container_image",
  "container_push",
)

container_image(
  name = "image",
  base = "//images:kubectl_base",
  cmd = ["/manager"],
  directory = "/",
  files = ["//dashboard-operator/cmd/manager"],
  tars = ["//dashboard-operator/channels:tar"],
)

container_push(
  name = "push-image",
  format = "Docker",
  image = ":image",
  registry = "{STABLE_DOCKER_REGISTRY}",
  repository = "{STABLE_PROJECT_NAME}/dashboard-operator",
  stamp = True,
  tag = "{STABLE_DOCKER_TAG}",
)
EOF
```

### Manifests & RBAC

We need a simple deployment to run our operator, and we want to run it under a
tightly-scoped RBAC role.

RBAC is the real pain-point here - we end up with a lot of permissions:
* The operator needs RBAC rules to see the CRDs.
* It needs permission to get / create / update the Deployments and other types
  that it is managing
* It needs permission to create the ClusterRoles / Roles that the dashboard
  needs
* Because of that, we also need permissions for all the permissions we are going
  to create.

The last one in particular can result in a non-trivial RBAC policy.  My approach:

* Start with minimal permissions (just watching addons.k8s.io dashboards), and
  then add permissions iteratively
* If you're going to allow list, I tend to just allow get, list and watch -
  there's not a huge security reason to treat them separately as far as I can
  see
* Similarly I treat create and patch together
* No controller should be using update (because of version skew issues), so I
  tend to grant that one begrudgingly
* The RBAC policy in the manifest may scope down the permissions even more (for
  example scoping to resourceNames), in which case we can - and should - copy
  it.  That's what we did here for dashboard.

TODO: What happens when the next version of dashboard needs more permissions?

For dashboard, the iterative cycle looks like:
* `kubectl logs dashboard-operator<tab>`
* add permissions to yaml files
* `kubectl apply -f k8s/`
* `kubectl delete pod -l component=dashboard-operator`
* repeat!

The results are in the k8s/ folder of dashboard-operator/

Once you get the RBAC permissions correct, the procedure to run is:

```
bazel run :push-image
kubectl apply -f k8s/
```

## Manifest simplification: Automatic labels

Similar to how kustomize works, often you won't want labels hard-coded in the
manifest, but will use them to distinguish multiple instances.  Even if you're
writing something you expect to be a singleton instance, it can be tedious and
error-prone to specify labels on every object in the manifest.

Instead, the StandardReconciler can add labels to every object in the manifest:

```go
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
       // TODO: Dynamic labels?
       labels := map[string]string{
               "k8s-app": "kubernetes-dashboard",
       }

       r := &ReconcileDashboard{}
       r.Reconciler.Init(mgr, &api.Dashboard{}, "dashboard",
               operators.WithObjectTransform(operators.AddLabels(labels)),
...

```

NOTE: operators.AddLabels does not _yet_ add selectors to Deployments /
Daemonsets, nor to the templates.  But - like kustomize - this should happen
real-soon-now.  TODO

By adding the above label transformation, we can remove the labels from the
manifest.

## Manifest simplification: Automatic namespace

The operator framework automatically creates objects in the same namespace as
the CR (by specifying the namespace to kubectl).  As such, we can remove the
namespaces from the manifest.

NOTE: We don't currently apply the namespace within objects.  For example, we
don't set the namespace on a RoleBinding subjects.namespace.  However, it seems
that most objects default to the same namespace - but presumably
ClusterRoleBinding will not.  TODO

NOTE: For non-namespaces objects (ClusterRole and ClusterRoleBinding), we often
need to name them with the namespace to support multiple instances.  We
currently do this for istio using a string-substitution hack.  TODO

### Customize Application

The operator framework automatically creates an 
[application](https://github.com/kubernetes-sigs/application) 
instance for each CR. The application can contain human readable information
in addition to deployment status that can be surfaced in various user interfaces 
(e.g. Google Cloud Console). 

You can pass in a base application object to provide information for your addon:

```go
	import applicationv1beta1 "github.com/kubernetes-sigs/application/pkg/apis/app/v1beta1"
	// ..
	app := applicationv1beta1.Application{
		Spec: applicationv1beta1.ApplicationSpec{
			Descriptor: applicationv1beta1.Descriptor{
				Description: "Kubernetes Dashboard is a general purpose, web-based UI for Kubernetes clusters. It allows users to manage applications running in the cluster and troubleshoot them, as well as manage the cluster itself.",
				Links:       []applicationv1beta1.Link{{Description: "Project Homepage", Url: "https://github.com/kubernetes/dashboard"}},
				Keywords:    []string{"dashboard", "addon"},
			},
		},
	}
	
	r.Reconciler.Init(mgr, &api.KubeDNS{}, "kubedns",
		operators.WithGroupVersionKind(api.SchemeGroupVersion.WithKind("dashboard")),
 		operators.WithApplication(app),
 	)

```

### Next steps

* Read about [adding tests](tests.md)
* Remove cruft from the manifest yaml (Namespaces, Names, Labels)
* Split up this document into sections?

