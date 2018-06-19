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

/*
Package admission provides methods to ensure the webhooks can have
proper CA and certificate to work correctly.

There are 3 typical ways to use this library:

* Deploying as a controller. Sync method can easily wrapped as a Reconcile method.

* Invoking it as a library in the webhook server before starting to serve the traffic.

* Deploying it as an init container along with the webhook server.

Webhook Configuration

The following is an example MutatingWebhookConfiguration in yaml.

	apiVersion: admissionregistration.k8s.io/v1beta1
	kind: MutatingWebhookConfiguration
	metadata:
	  name: myMutatingWebhookConfiguration
	  annotations:
	    secret.certprovisioner.kubernetes.io/webhook-1: namespace-bar/secret-foo
	    secret.certprovisioner.kubernetes.io/webhook-2: default/secret-baz
	webhooks:
	- name: webhook-1
	  rules:
	  - apiGroups:
		- ""
		apiVersions:
		- v1
		operations:
		- "*"
		resources:
		- pods
	  clientConfig:
		service:
		  namespace: service-ns-1
		  name: service-foo
		  path: "/mutating-pods"
		caBundle: [] # CA bundle here
	- name: webhook-2
	  rules:
	  - apiGroups:
		- apps
		apiVersions:
		- v1
		operations:
		- "*"
		resources:
		- deployments
	  clientConfig:
		service:
		  namespace: service-ns-2
		  name: service-bar
		  path: "/mutating-deployment"
		caBundle: [] # CA bundle here

Build the CertProvisioner

You can choose to provide your own CertGenerator and CertWriterProvider.
An easier way is to use an empty Options the package will default it with reasonable values.
The package will write self-signed certificates to secrets.

	// Build a CertProvisioner with unspecified CertGenerator and CertWriter.
	cp := &CertProvisioner{client: getClientOrDie()}

Provision certificates

Provision certificates for webhook configuration objects' by calling Sync method.

	err = cp.Sync(mwc)
	if err != nil {
		// handler error
	}

If the above MutatingWebhookConfiguration is processed, the cert provisioner will provision
the certificate and create an secret named "secret-foo" in namespace "namespace-bar" for webhook "webhook-1".
Similarly, it will create an secret named "secret-baz" in namespace "default" for webhook "webhook-2".
And it will also write the CA back to the WebhookConfiguration.
*/
package admission
