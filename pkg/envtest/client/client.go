package client

import (
	"fmt"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kcapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	envtestName = "envtest"
)

// KubeConfigFromREST reverse-engineers a kubeconfig file from a rest.Config.
// The options are tailored towards the rest.Configs we generate, so they're
// not broadly applicable.
//
// This is not intended to be exposed outside envtest for the above reasons.
func KubeConfigFromREST(cfg *rest.Config) ([]byte, error) {
	kubeConfig := kcapi.NewConfig()
	protocol := "https"
	if !rest.IsConfigTransportTLS(*cfg) {
		protocol = "http"
	}

	// cfg.Host is a URL, so we need to parse it so we can properly append the API path
	baseURL, err := url.Parse(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("unable to interpret config's host value as a URL: %w", err)
	}

	kubeConfig.Clusters[envtestName] = &kcapi.Cluster{
		// TODO(directxman12): if client-go ever decides to expose defaultServerUrlFor(config),
		// we can just use that.  Note that this is not the same as the public DefaultServerURL,
		// which requires us to pass a bunch of stuff in manually.
		Server:                   (&url.URL{Scheme: protocol, Host: baseURL.Host, Path: cfg.APIPath}).String(),
		CertificateAuthorityData: cfg.CAData,
	}
	kubeConfig.AuthInfos[envtestName] = &kcapi.AuthInfo{
		// try to cover all auth strategies that aren't plugins
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
		Token:                 cfg.BearerToken,
		Username:              cfg.Username,
		Password:              cfg.Password,
	}
	kcCtx := kcapi.NewContext()
	kcCtx.Cluster = envtestName
	kcCtx.AuthInfo = envtestName
	kubeConfig.Contexts[envtestName] = kcCtx
	kubeConfig.CurrentContext = envtestName

	contents, err := clientcmd.Write(*kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to serialize kubeconfig file: %w", err)
	}
	return contents, nil
}
