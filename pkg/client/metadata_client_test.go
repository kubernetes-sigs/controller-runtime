package client

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakemetadata "k8s.io/client-go/metadata/fake"
)

var (
	testScheme *runtime.Scheme
	testMapper meta.RESTMapper
)

func init() {
	testScheme = runtime.NewScheme()
	_ = appsv1.AddToScheme(testScheme)
	_ = metav1.AddMetaToScheme(testScheme)

	testMapper = testrestmapper.TestOnlyStaticRESTMapper(testScheme)
}

func newPartialObjectMetadata(apiVersion, kind, namespace, name string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func TestMetadataClient_list(t *testing.T) {
	client := fakemetadata.NewSimpleMetadataClient(testScheme,
		newPartialObjectMetadata(appsv1.SchemeGroupVersion.String(), "Deployment", "default", "deploy1"),
	)
	metadataClient := metadataClient{client: client, restMapper: testMapper}

	listGVK := appsv1.SchemeGroupVersion.WithKind("DeploymentList")
	list := &metav1.PartialObjectMetadataList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: listGVK.GroupVersion().String(),
			Kind:       listGVK.Kind,
		},
	}

	if err := metadataClient.List(context.TODO(), list); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected items length is 1, got: %v", len(list.Items))
	}
	if list.GroupVersionKind() != listGVK {
		t.Errorf("expected %v, got %v", listGVK, list.GroupVersionKind())
	}
}
