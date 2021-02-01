package main

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	v1 "sigs.k8s.io/controller-runtime/examples/applyconfig/api/v1"
	// To use the newly generated types, import the ApplyConfiguration types
	// These are generated based on the structs provided in v1 above
	ac "sigs.k8s.io/controller-runtime/examples/applyconfig/api/v1/ac"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

var sch *runtime.Scheme

func makePtrForString(s string) *string {
	return &s
}

func runMain() int {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	env := &envtest.Environment{
		CRDDirectoryPaths: []string{"api/v1/."},
	}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}
	defer env.Stop()

	sch = runtime.NewScheme()
	clientgoscheme.AddToScheme(sch)

	// The SSA typed client does not change the scaffolding code.
	// The types defined must still be registered in the normal way
	bld := &scheme.Builder{GroupVersion: schema.GroupVersion{
		Group:   "applytest.kubebuilder.io",
		Version: "v1",
	}}
	bld.Register(&v1.Foo{})
	bld.AddToScheme(sch)

	cl, err := client.New(cfg, client.Options{Scheme: sch})
	if err != nil {
		panic(err)
	}

	// Create two owners for two apply clients
	var owner client.FieldOwner
	owner = "fieldmanager_test"
	var owner2 client.FieldOwner
	owner2 = "fieldmanager_test2"

	// ====================================================
	// Without ApplyConfigurations
	// Please skip to the next section to see a working example with ApplyConfigurations
	// This example is used to show the limitations of the current structs
	// ====================================================

	apply1 := &v1.Foo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Foo",
			APIVersion: "applytest.kubebuilder.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "normalobject",
			Namespace: "default",
		},
		NonNullableField: "test",
	}

	// Suppose the first apply is executed and we want to modify a field not owned by the first apply (NullableField)
	// This apply will fail because NonNullableField is defaulted to "" and the second applier will attempt to own
	// the NonNullableField despite leaving it empty. Since the first applier already owns the field, the apply will fail
	apply2 := &v1.Foo{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Foo",
			APIVersion: "applytest.kubebuilder.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "normalobject",
			Namespace: "default",
		},
		NullableField: "value",
	}

	if err := cl.Patch(context.TODO(), apply1, client.Apply, owner); err != nil {
		panic(err)
	}

	if err := cl.Patch(context.TODO(), apply2, client.Apply, owner2); err == nil {
		fmt.Errorf("Expected a conflict but no conflict was hit")
	}

	// ====================================================
	// Using ApplyConfigurations Structs
	// ====================================================

	fooObj := &v1.Foo{}

	// To use the SSA typed client, use the ApplyConfiguration generated types instead of the types defined in v1
	applyConfig1 := &ac.FooApplyConfiguration{
		TypeMetaApplyConfiguration:   metav1ac.TypeMetaApplyConfiguration{Kind: makePtrForString("Foo"), APIVersion: makePtrForString("applytest.kubebuilder.io/v1")},
		ObjectMetaApplyConfiguration: metav1ac.ObjectMeta().WithName("acdefault").WithNamespace("default"),
	}

	applyConfig1 = applyConfig1.WithNonNullableField("value1")

	// Without ApplyConfiguration, NonNullableField is not an optional field so it must be specified if Foo{} was used instead of the ApplyConfiguration
	// The ApplyConfiguration uses pointers for all objects, so we can set a nil value to indicate that a field is not of interest in the Apply
	applyConfig2 := &ac.FooApplyConfiguration{
		TypeMetaApplyConfiguration:   metav1ac.TypeMetaApplyConfiguration{Kind: makePtrForString("Foo"), APIVersion: makePtrForString("applytest.kubebuilder.io/v1")},
		ObjectMetaApplyConfiguration: metav1ac.ObjectMeta().WithName("acdefault").WithNamespace("default"),
		NonNullableField:             nil,
		NullableField:                makePtrForString("value2"),
	}

	if err := cl.Patch(context.TODO(), fooObj, client.Apply, owner, client.ApplyFrom(applyConfig1)); err != nil {
		panic(err)
	}

	if err := cl.Patch(context.TODO(), fooObj, client.Apply, owner2, client.ApplyFrom(applyConfig2)); err != nil {
		panic(err)
	}
	fooObj = &v1.Foo{}
	cl.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "acdefault"}, fooObj)
	fmt.Printf("Object NonNullableField: %s\n", fooObj.NonNullableField)
	fmt.Printf("Object NullableField: %s\n", fooObj.NullableField)
	fmt.Println("Managed Fields:", fooObj.ObjectMeta.ManagedFields)

	return 0
}

func main() {
	os.Exit(runMain())
}
