package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type CommonObject interface {
	runtime.Object
	metav1.Object
	ComponentName() string
	CommonSpec() CommonSpec
	GetCommonStatus() CommonStatus
	SetCommonStatus(CommonStatus)
}

type CommonSpec struct {
	Version string `json:"version,omitempty"`
	Channel string `json:"channel,omitempty"`
}

//go:generate go run ../../../../../../vendor/k8s.io/code-generator/cmd/deepcopy-gen/main.go -O zz_generated.deepcopy -i ./... -h ../../../../../../hack/boilerplate.go.txt

// +k8s:deepcopy-gen=true
type CommonStatus struct {
	Healthy bool     `json:"healthy"`
	Errors  []string `json:"errors,omitempty"`
}
