//go:generate controller-gen object crd paths="." output:dir="."
// +groupName=applytest.kubebuilder.io
// +versionName=v1
//go:generate $GOPATH/src/sigs.k8s.io/controller-tools/controller-gen apply paths="./..."
//go:generate $GOPATH/src/sigs.k8s.io/controller-tools/controller-gen object paths="./..."
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:generate=true
// +kubebuilder:object:root=true
// +kubebuilder:ac:root=true
type Foo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	NonNullableField  string `json:"nonNullableField"`
	NullableField     string `json:"nullableField,omitempty"`
}
