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

package certwriter

import (
	"bytes"
	"fmt"
	"net/url"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"crypto/tls"

	"sigs.k8s.io/controller-runtime/pkg/admission/certgenerator"
)

const (
	// CACertName is the name of the CA certificate
	CACertName = "ca-cert.pem"
	// ServerKeyName is the name of the server private key
	ServerKeyName = "key.pem"
	// ServerCertName is the name of the serving certificate
	ServerCertName = "cert.pem"
)

// CertWriter provides method to handle webhooks.
type CertWriter interface {
	// EnsureCert ensures that the webhooks it manages have the right certificates.
	EnsureCert() error
}

func handleCommon(webhook *admissionregistrationv1beta1.Webhook, ch certReadWriter) error {
	webhookName := webhook.Name
	certs, err := ch.read(webhookName)
	if apierrors.IsNotFound(err) {
		certs, err = ch.write(webhookName)
		switch {
		case apierrors.IsAlreadyExists(err):
			certs, err = ch.read(webhookName)
			if err != nil {
				return err
			}
		case err != nil:
			return err
		}
	} else if err != nil {
		return err
	}

	// Recreate the cert if it's invalid.
	if !validCert(certs) {
		certs, err = ch.overwrite(webhookName)
		if err != nil {
			return err
		}
	}

	// Ensure the CA bundle in the webhook configuration has the signing CA.
	caBundle := webhook.ClientConfig.CABundle
	caCert := certs.CACert
	if !bytes.Contains(caBundle, caCert) {
		webhook.ClientConfig.CABundle = append(caBundle, caCert...)
	}
	return nil
}

// certReadWriter provides methods for handling certificates for webhook.
type certReadWriter interface {
	// read reads a wehbook name and returns the certs for it.
	read(webhookName string) (*certgenerator.CertArtifacts, error)
	// write writes the certs and return the certs it wrote.
	write(webhookName string) (*certgenerator.CertArtifacts, error)
	// overwrite overwrites the existing certs and return the certs it wrote.
	overwrite(webhookName string) (*certgenerator.CertArtifacts, error)
}

func validCert(certs *certgenerator.CertArtifacts) bool {
	// TODO:
	// 1) validate the key and the cert are valid pair e.g. call crypto/tls.X509KeyPair()
	// 2) validate the cert with the CA cert
	// 3) validate the cert is for a certain DNSName
	// e.g.
	// c, err := tls.X509KeyPair(cert, key)
	// err := c.Verify(options)
	if certs == nil {
		return false
	}
	_, err := tls.X509KeyPair(certs.Cert, certs.Key)
	return err == nil
}

func getWebhooksFromObject(obj runtime.Object) ([]admissionregistrationv1beta1.Webhook, error) {
	switch typed := obj.(type) {
	case *admissionregistrationv1beta1.MutatingWebhookConfiguration:
		return typed.Webhooks, nil
	case *admissionregistrationv1beta1.ValidatingWebhookConfiguration:
		return typed.Webhooks, nil
	//case *unstructured.Unstructured:
	// TODO: implement this if needed
	default:
		return nil, fmt.Errorf("unsupported type: %T, only support v1beta1.MutatingWebhookConfiguration and v1beta1.ValidatingWebhookConfiguration", typed)
	}
}

func webhookClientConfigToCommonName(config *admissionregistrationv1beta1.WebhookClientConfig) (string, error) {
	if config.Service != nil && config.URL != nil {
		return "", fmt.Errorf("service and URL can't be set at the same time in a webhook: %v", config)
	}
	if config.Service == nil && config.URL == nil {
		return "", fmt.Errorf("one of service and URL need to be set in a webhook: %v", config)
	}
	if config.Service != nil {
		return certgenerator.ServiceToCommonName(config.Service.Namespace, config.Service.Name), nil
	}
	if config.URL != nil {
		u, err := url.Parse(*config.URL)
		return u.Host, err
	}
	return "", nil
}
