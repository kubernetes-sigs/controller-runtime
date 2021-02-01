module sigs.k8s.io/controller-runtime/examples/applyconfig

go 1.15

require (
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v0.21.0
	sigs.k8s.io/controller-runtime v0.8.1
)

replace sigs.k8s.io/controller-runtime => ../..
