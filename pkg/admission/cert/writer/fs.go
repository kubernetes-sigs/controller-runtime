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

package writer

//import (
//	"fmt"
//	"strings"
//
//	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
//	"k8s.io/apimachinery/pkg/api/meta"
//	"k8s.io/apimachinery/pkg/runtime"
//	"sigs.k8s.io/controller-runtime/pkg/admission/certgenerator"
//)
//
//const (
//	// FSCertProvisionAnnotationKeyPrefix should be used in an annotation in the following format:
//	// fs.certprovisioner.kubernetes.io/<webhook-name>: path/to/certs/
//	// the webhook cert manager library will provision the certificate for the webhook by
//	// storing it under the specified path.
//	// format: local.certprovisioner.kubernetes.io/webhookName: path/to/certs/
//	FSCertProvisionAnnotationKeyPrefix = "fs.certprovisioner.kubernetes.io/"
//)
//
//// FSCertWriterProvider deals with writing to the local filesystem.
//type FSCertWriterProvider struct {
//	CertGenerator certgenerator.CertGenerator
//}
//
//var _ CertWriter = &FSCertWriterProvider{}
//
//// Provide creates a new CertWriter and initialized with the passed-in webhookConfig.
//func (s *FSCertWriterProvider) Provide(webhookConfig runtime.Object) (CertWriter, error) {
//	fsWebhookMap := map[string]*webhookAndPath{}
//
//	accessor, err := meta.Accessor(webhookConfig)
//	if err != nil {
//		return nil, err
//	}
//	annotations := accessor.GetAnnotations()
//	if annotations == nil {
//		return nil, nil
//	}
//
//	// Parse the annotations to extract info
//	for k, v := range annotations {
//      // an example annotation: local.certprovisioner.kubernetes.io/webhookName: path/to/certs/
//		if strings.HasPrefix(k, FSCertProvisionAnnotationKeyPrefix) {
//			webhookName := strings.TrimPrefix(k, FSCertProvisionAnnotationKeyPrefix)
//			fsWebhookMap[webhookName] = &webhookAndPath{
//				path: v,
//			}
//		}
//	}
//
//	webhooks, err := getWebhooksFromObject(webhookConfig)
//	if err != nil {
//		return nil, err
//	}
//	for i, webhook := range webhooks {
//		if p, found := fsWebhookMap[webhook.Name]; found {
//			p.webhook = &webhooks[i]
//		}
//	}
//
//	// validation
//	for k, v := range fsWebhookMap {
//		if v.webhook == nil {
//			return nil, fmt.Errorf("expecting a webhook named %q", k)
//		}
//	}
//
//	generator := s.CertGenerator
//	if s.CertGenerator == nil {
//		generator = &certgenerator.SelfSignedCertGenerator{}
//	}
//
//	return &fsCertWriter{
//		CertGenerator: generator,
//		webhookConfig: webhookConfig,
//		webhookToPath: fsWebhookMap,
//	}, nil
//}
//
//// fsCertWriter deals with writing to the local filesystem.
//type fsCertWriter struct {
//	CertGenerator certgenerator.CertGenerator
//
//	webhookConfig runtime.Object
//	webhookToPath map[string]*webhookAndPath
//}
//
//type webhookAndPath struct {
//	webhook *admissionregistrationv1beta1.Webhook
//	path    string
//}
//
//var _ certReadWriter = &fsCertWriter{}
//
//// EnsureCert processes the webhooks managed by this CertWriter.
//// It provisions the certificate and update the CA in the webhook.
//// It will write the certificate to the filesystem.
//func (s *fsCertWriter) EnsureCert() error {
//	var err error
//	for _, v := range s.webhookToPath {
//		err = handleCommon(v.webhook, s)
//		if err != nil {
//			return err
//		}
//	}
//	return nil
//}
//
//func (s *fsCertWriter) write(webhookName string) (*certgenerator.CertArtifacts, error) {
//	// TODO: implement this
//	return nil, nil
//}
//
//func (s *fsCertWriter) overwrite(webhookName string) (*certgenerator.CertArtifacts, error) {
//	// TODO: implement this
//	return nil, nil
//}
//
//func (s *fsCertWriter) read(webhookName string) (*certgenerator.CertArtifacts, error) {
//	// TODO: implement this
//	return nil, nil
//}
