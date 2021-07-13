package testingclient

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Verb string

const (
	// Verb values are chosen to match those returned by client-go testing.Action#GetVerb()
	GetVerb       Verb = "get"
	CreateVerb    Verb = "create"
	DeleteVerb    Verb = "delete"
	UpdateVerb    Verb = "update"
	PatchVerb     Verb = "patch"
	ListVerb      Verb = "list"
	DeleteAllVerb Verb = "delete-collection"
	WatchVerb     Verb = "watch"

	// AnyVerb is used to match any of the above API verbs.
	AnyVerb Verb = "*"
)

var (
	// AnyKind is a sentinel value used as a wildcard to match any Kind.
	AnyKind = &unstructured.Unstructured{}
	// AnyObject is an empty ObjectKey used to match an object of any identity.
	AnyObject = client.ObjectKey{}

	// anyKindGVK is used for internal representation of AnyKind.
	anyKindGVK = schema.GroupVersionKind{Kind: "*"}
)

type resourceActionKey struct {
	verb      Verb
	kind      schema.GroupVersionKind
	objectKey client.ObjectKey
}
