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
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"

	"k8s.io/api/admissionregistration/v1beta1"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/writer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

// setDefault does defaulting for the Server.
func (s *Server) setDefault() {
	s.setServerDefault()
	s.setBootstrappingDefault()
}

// setServerDefault does defaulting for the ServerOptions.
func (s *Server) setServerDefault() {
	if len(s.Name) == 0 {
		s.Name = "default-k8s-webhook-server"
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
		s.CertDir = path.Join("k8s-webhook-server", "cert")
	}

	if s.Client == nil {
		cfg, err := config.GetConfig()
		if err != nil {
			s.err = err
			return
		}
		s.Client, err = client.New(cfg, client.Options{})
		if err != nil {
			s.err = err
			return
		}
	}
}

// setBootstrappingDefault does defaulting for the Server bootstrapping.
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

	if s.Writer == nil {
		s.Writer = os.Stdout
	}
	if s.GenerateManifests {
		s.Client = &writerClient{
			writer: s.Writer,
		}
	}

	var certWriter writer.CertWriter
	var err error
	if s.Secret != nil {
		certWriter, err = writer.NewSecretCertWriter(
			writer.SecretCertWriterOptions{
				Secret: s.Secret,
				Client: s.Client,
			})
	} else {
		certWriter, err = writer.NewFSCertWriter(
			writer.FSCertWriterOptions{
				Path: s.CertDir,
			})
	}
	if err != nil {
		s.err = err
		return
	}
	s.certProvisioner = &cert.Provisioner{
		CertWriter: certWriter,
	}
}

// installWebhookConfig writes the configuration of admissionWebhookConfiguration in yaml format
// if GenerateManifests is true.
// Otherwise, it creates the the admissionWebhookConfiguration objects and service if any.
// It also provisions the certificate for the admission server.
func (s *Server) installWebhookConfig() error {
	// do defaulting if necessary
	s.once.Do(s.setDefault)
	if s.err != nil {
		return s.err
	}

	if !s.GenerateManifests && s.DisableInstaller {
		log.Info("webhook installer is disabled")
		return nil
	}
	if s.GenerateManifests {
		log.Info("generating webhook related manifests")
	}

	var err error
	s.webhookConfigurations, err = s.whConfigs()
	if err != nil {
		return err
	}
	svc := s.service()
	objects := append(s.webhookConfigurations, svc)

	cc, err := s.getClientConfig()
	if err != nil {
		return err
	}
	// Provision the cert by creating new one or refreshing existing one.
	_, err = s.certProvisioner.Provision(cert.Options{
		ClientConfig: cc,
		Objects:      s.webhookConfigurations,
	})
	if err != nil {
		return err
	}

	if s.GenerateManifests {
		objects = append(objects, s.deployment())
	}

	return batchCreateOrReplace(s.Client, objects...)
}

func (s *Server) getClientConfig() (*admissionregistration.WebhookClientConfig, error) {
	if s.Host != nil && s.Service != nil {
		return nil, errors.New("URL and Service can't be set at the same time")
	}
	cc := &admissionregistration.WebhookClientConfig{
		CABundle: []byte{},
	}
	if s.Host != nil {
		u := url.URL{
			Scheme: "https",
			Host:   net.JoinHostPort(*s.Host, strconv.Itoa(int(s.Port))),
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

// getClientConfigWithPath constructs a WebhookClientConfig based on the server options.
// It will use path to the set the path in WebhookClientConfig.
func (s *Server) getClientConfigWithPath(path string) (*admissionregistration.WebhookClientConfig, error) {
	cc, err := s.getClientConfig()
	if err != nil {
		return nil, err
	}
	return cc, setPath(cc, path)
}

// setPath sets the path in the WebhookClientConfig.
func setPath(cc *admissionregistration.WebhookClientConfig, path string) error {
	if cc.URL != nil {
		u, err := url.Parse(*cc.URL)
		if err != nil {
			return err
		}
		u.Path = path
		urlString := u.String()
		cc.URL = &urlString
	}
	if cc.Service != nil {
		cc.Service.Path = &path
	}
	return nil
}

// whConfigs creates a mutatingWebhookConfiguration and(or) a validatingWebhookConfiguration based on registry.
// For the same type of webhook configuration, it generates a webhook entry per endpoint.
func (s *Server) whConfigs() ([]runtime.Object, error) {
	objs := []runtime.Object{}
	mutatingWH, err := s.mutatingWHConfigs()
	if err != nil {
		return nil, err
	}
	if mutatingWH != nil {
		objs = append(objs, mutatingWH)
	}
	validatingWH, err := s.validatingWHConfigs()
	if err != nil {
		return nil, err
	}
	if validatingWH != nil {
		objs = append(objs, validatingWH)
	}
	return objs, nil
}

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
				APIVersion: fmt.Sprintf("%s/%s", admissionregistration.GroupName, "v1beta1"),
				Kind:       "MutatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: s.MutatingWebhookConfigName,
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
				APIVersion: fmt.Sprintf("%s/%s", admissionregistration.GroupName, "v1beta1"),
				Kind:       "ValidatingWebhookConfiguration",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: s.ValidatingWebhookConfigName,
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
	cc, err := s.getClientConfigWithPath(path)
	if err != nil {
		return nil, err
	}
	webhook.ClientConfig = *cc
	return webhook, nil
}

// service creates a corev1.service object fronting the admission server.
func (s *Server) service() runtime.Object {
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
					// When using service, kube-apiserver will send admission request to port 443.
					Port:       443,
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: s.Port},
				},
			},
		},
	}
	return svc
}

func (s *Server) deployment() runtime.Object {
	namespace := "default"
	if s.Secret != nil {
		namespace = s.Secret.Namespace
	}

	labels := map[string]string{
		"app": "webhook-server",
	}
	if s.Service != nil {
		for k, v := range s.Service.Selectors {
			labels[k] = v
		}
	}

	image := "webhook-server:latest"
	if len(os.Getenv("IMG")) != 0 {
		image = os.Getenv("IMG")
	}

	readOnly := false
	if s.Service != nil {
		readOnly = true
	}

	var volumeSource corev1.VolumeSource
	if s.Secret != nil {
		var mode int32 = 420
		volumeSource = corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &mode,
				SecretName:  s.Secret.Name,
			},
		}
	} else {
		volumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webhook-server",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "webhook-server-container",
							Image:           image,
							ImagePullPolicy: corev1.PullAlways,
							Command: []string{
								"/bin/sh",
								"-c",
								"exec ./manager --disable-installer",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "webhook-server",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: s.Port,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "cert",
									MountPath: s.CertDir,
									ReadOnly:  readOnly,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name:         "cert",
							VolumeSource: volumeSource,
						},
					},
				},
			},
		},
	}
}
