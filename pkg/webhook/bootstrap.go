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

package webhook

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"

	"github.com/ghodss/yaml"

	"k8s.io/api/admissionregistration/v1beta1"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/writer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

// setDefault does defaulting for the Server.
func (s *Server) setDefault() {
	if len(s.Name) == 0 {
		s.Name = "default-admission-server"
	}
	if s.registry == nil {
		s.registry = map[string]Webhook{}
	}
	if s.sMux == nil {
		s.sMux = http.DefaultServeMux
	}
	if s.Port <= 0 {
		s.Port = 443
	}
	if len(s.CertDir) == 0 {
		s.CertDir = path.Join("admission-server", "cert")
	}
	s.setBootstrappingDefault()
}

// setBootstrappingDefault does defaulting for the Server  bootstrapping.
func (s *Server) setBootstrappingDefault() {
	if len(s.MutatingWebhookConfigName) == 0 {
		s.MutatingWebhookConfigName = "mutating-webhook-configuration"
	}
	if len(s.ValidatingWebhookConfigName) == 0 {
		s.ValidatingWebhookConfigName = "validating-webhook-configuration"
	}
	if s.Host == nil && s.Service == nil {
		varString := "localhost"
		s.Host = &varString
	}
	// Override the user's setting to use port 443 until
	// https://github.com/kubernetes/kubernetes/issues/67468 is resolved.
	if s.Service != nil && s.Port != 443 {
		s.Port = 443
	}
	if s.CertProvisioner == nil {
		s.CertProvisioner = &cert.Provisioner{
			Client: s.Client,
		}
	}
}

// bootstrap returns the configuration of admissionWebhookConfiguration in yaml format if dryrun is true.
// Otherwise, it creates the the admissionWebhookConfiguration objects and service if any.
// It also provisions the certificate for your admission server.
func (s *Server) bootstrap(dryrun bool) ([]byte, error) {
	s.once.Do(s.setDefault)

	var err error
	s.mutatingWebhookConfig, err = s.mutatingWHConfigs()
	if err != nil {
		return nil, err
	}
	err = s.CertProvisioner.Sync(s.mutatingWebhookConfig)
	if err != nil {
		return nil, err
	}

	s.validatingWebhookConfig, err = s.validatingWHConfigs()
	if err != nil {
		return nil, err
	}
	err = s.CertProvisioner.Sync(s.validatingWebhookConfig)
	if err != nil {
		return nil, err
	}

	svc := s.service()

	// if dryrun, return the AdmissionWebhookConfiguration in yaml format.
	if dryrun {
		return s.genYamlConfig([]runtime.Object{s.mutatingWebhookConfig, s.validatingWebhookConfig, svc})
	}

	if s.Client == nil {
		return nil, errors.New("client needs to be set for bootstrapping")
	}

	err = createOrReplace(s.Client, svc, func(current, desired runtime.Object) error {
		typedC := current.(*corev1.Service)
		typedD := desired.(*corev1.Service)
		typedC.Spec.Selector = typedD.Spec.Selector
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = createOrReplace(s.Client, s.mutatingWebhookConfig, func(current, desired runtime.Object) error {
		typedC := current.(*admissionregistration.MutatingWebhookConfiguration)
		typedD := desired.(*admissionregistration.MutatingWebhookConfiguration)
		typedC.Webhooks = typedD.Webhooks
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = createOrReplace(s.Client, s.validatingWebhookConfig, func(current, desired runtime.Object) error {
		typedC := current.(*admissionregistration.ValidatingWebhookConfiguration)
		typedD := desired.(*admissionregistration.ValidatingWebhookConfiguration)
		typedC.Webhooks = typedD.Webhooks
		return nil
	})
	if err != nil {
		return nil, err
	}

	return nil, nil
}

type mutateFn func(current, desired runtime.Object) error

// createOrReplace creates the object if it doesn't exist;
// otherwise, it will replace it.
// When replacing, fn knows how to preserve existing fields in the object GET from the APIServer.
// TODO: use the helper in #98 when it merges.
func createOrReplace(c client.Client, obj runtime.Object, fn mutateFn) error {
	if obj == nil {
		return nil
	}
	err := c.Create(context.Background(), obj)
	if apierrors.IsAlreadyExists(err) {
		// TODO: retry mutiple times with backoff if necessary.
		existing := obj.DeepCopyObject()
		objectKey, err := client.ObjectKeyFromObject(obj)
		if err != nil {
			return err
		}
		err = c.Get(context.Background(), objectKey, existing)
		if err != nil {
			return err
		}
		err = fn(existing, obj)
		if err != nil {
			return err
		}
		return c.Update(context.Background(), existing)
	}
	return err
}

// genYamlConfig generates yaml config for runtime.Object
func (s *Server) genYamlConfig(objs []runtime.Object) ([]byte, error) {
	var buf bytes.Buffer
	firstObject := true
	for _, awc := range objs {
		if !firstObject {
			_, err := buf.WriteString("---")
			if err != nil {
				return nil, err
			}
		}
		firstObject = false
		b, err := yaml.Marshal(awc)
		if err != nil {
			return nil, err
		}
		_, err = buf.Write(b)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (s *Server) annotations(t types.WebhookType) map[string]string {
	anno := map[string]string{}
	for _, webhook := range s.registry {
		if webhook.GetType() != t {
			continue
		}
		if s.Secret != nil {
			key := writer.SecretCertProvisionAnnotationKeyPrefix + webhook.GetName()
			anno[key] = s.Secret.String()
		} else {
			key := writer.FSCertProvisionAnnotationKeyPrefix + webhook.GetName()
			anno[key] = s.CertDir
		}
	}
	return anno
}

type webhookClientConfig struct {
	*admissionregistration.WebhookClientConfig
}

func (s *Server) clientConfig() (*webhookClientConfig, error) {
	if s.Host != nil && s.Service != nil {
		return nil, errors.New("URL and Service can't be set at the same time")
	}
	cc := &webhookClientConfig{
		WebhookClientConfig: &admissionregistration.WebhookClientConfig{
			CABundle: []byte{},
		},
	}
	if s.Host != nil {
		u := url.URL{
			Scheme: "https",
			Host:   *s.Host,
		}
		urlString := u.String()
		cc.URL = &urlString
	}
	if s.Service != nil {
		cc.Service = &admissionregistration.ServiceReference{
			Name:      s.Service.Name,
			Namespace: s.Service.Namespace,
			// Path will be set later
		}
	}
	return cc, nil
}

func (w *webhookClientConfig) withPath(path string) error {
	if w.URL != nil {
		u, err := url.Parse(*w.URL)
		if err != nil {
			return err
		}
		u.Path = path
		urlString := u.String()
		w.URL = &urlString
	}
	if w.Service != nil {
		w.Service.Path = &path
	}
	return nil
}

// whConfigs creates a mutatingWebhookConfiguration and(or) a validatingWebhookConfiguration based on registry.
// For the same type of webhook configuration, it generates a webhook entry per endpoint.
func (s *Server) mutatingWHConfigs() (runtime.Object, error) {
	mutatingWebhooks := []v1beta1.Webhook{}
	for path, webhook := range s.registry {
		if webhook.GetType() != types.WebhookTypeMutating {
			continue
		}

		admissionWebhook := webhook.(*admission.Webhook)
		wh, err := s.admissionWebhook(path, admissionWebhook)
		if err != nil {
			return nil, err
		}
		mutatingWebhooks = append(mutatingWebhooks, *wh)

	}

	if len(mutatingWebhooks) > 0 {
		return &admissionregistration.MutatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "admissionregistration.k8s.io/v1beta1",
				Kind:       "MutatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        s.MutatingWebhookConfigName,
				Annotations: s.annotations(types.WebhookTypeMutating),
			},
			Webhooks: mutatingWebhooks,
		}, nil
	}
	return nil, nil
}

func (s *Server) validatingWHConfigs() (runtime.Object, error) {
	validatingWebhooks := []v1beta1.Webhook{}
	for path, webhook := range s.registry {
		var admissionWebhook *admission.Webhook
		if webhook.GetType() != types.WebhookTypeValidating {
			continue
		}

		admissionWebhook = webhook.(*admission.Webhook)
		wh, err := s.admissionWebhook(path, admissionWebhook)
		if err != nil {
			return nil, err
		}
		validatingWebhooks = append(validatingWebhooks, *wh)
	}

	if len(validatingWebhooks) > 0 {
		return &admissionregistration.ValidatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "admissionregistration.k8s.io/v1beta1",
				Kind:       "ValidatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        s.ValidatingWebhookConfigName,
				Annotations: s.annotations(types.WebhookTypeValidating),
			},
			Webhooks: validatingWebhooks,
		}, nil
	}
	return nil, nil
}

func (s *Server) admissionWebhook(path string, wh *admission.Webhook) (*admissionregistration.Webhook, error) {
	webhook := &admissionregistration.Webhook{
		Name:              wh.GetName(),
		Rules:             wh.Rules,
		FailurePolicy:     wh.FailurePolicy,
		NamespaceSelector: wh.NamespaceSelector,
		ClientConfig: admissionregistration.WebhookClientConfig{
			// The reason why we assign an empty byte array to CABundle is that
			// CABundle field will be updated by the Provisioner.
			CABundle: []byte{},
		},
	}
	cc, err := s.clientConfig()
	if err != nil {
		return nil, err
	}
	err = cc.withPath(path)
	if err != nil {
		return nil, err
	}
	webhook.ClientConfig = *cc.WebhookClientConfig
	return webhook, nil
}

// service creates a corev1.service object fronting the admission server.
func (s *Server) service() *corev1.Service {
	if s.Service == nil {
		return nil
	}
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Service.Name,
			Namespace: s.Service.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: s.Service.Selectors,
			Ports: []corev1.ServicePort{
				{
					Port: s.Port,
				},
			},
		},
	}
	return svc
}
