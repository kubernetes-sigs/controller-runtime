package admission

import (
	"testing"

	"k8s.io/api/admissionregistration/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCertProvisionerInit(t *testing.T) {
	p := &CertProvisioner{
		Client: fake.NewFakeClient(),
	}
	config := &v1beta1.MutatingWebhookConfiguration{}

	err := p.Sync(config)
	if err != nil {
		t.Fatalf("expect nil; got %q", err)
	}
}
