package internal

import (
	"fmt"
	"net/url"
)

func MakeAPIServerArgs(ps DefaultedProcessInput, etcdURL *url.URL) ([]string, error) {
	if etcdURL == nil {
		return []string{}, fmt.Errorf("must configure Etcd URL")
	}

	args := []string{
		"--authorization-mode=Node,RBAC",
		"--runtime-config=admissionregistration.k8s.io/v1alpha1",
		"--v=3", "--vmodule=",
		"--admission-control=Initializers,NamespaceLifecycle,LimitRanger,ServiceAccount,SecurityContextDeny,DefaultStorageClass,DefaultTolerationSeconds,ResourceQuota",
		"--admission-control-config-file=",
		"--bind-address=0.0.0.0",
		"--storage-backend=etcd3",
		fmt.Sprintf("--etcd-servers=%s", etcdURL.String()),
		fmt.Sprintf("--cert-dir=%s", ps.Dir),
		fmt.Sprintf("--insecure-port=%s", ps.URL.Port()),
		fmt.Sprintf("--insecure-bind-address=%s", ps.URL.Hostname()),
	}

	return args, nil
}

func GetAPIServerStartMessage(u url.URL) string {
	if isSecureScheme(u.Scheme) {
		// https://github.com/kubernetes/kubernetes/blob/5337ff8009d02fad613440912e540bb41e3a88b1/staging/src/k8s.io/apiserver/pkg/server/serve.go#L89
		return "Serving securely on " + u.Host
	}

	// https://github.com/kubernetes/kubernetes/blob/5337ff8009d02fad613440912e540bb41e3a88b1/pkg/kubeapiserver/server/insecure_handler.go#L121
	return "Serving insecurely on " + u.Host
}
