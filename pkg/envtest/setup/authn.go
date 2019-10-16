package setup

import (
	"sync"
	"fmt"
	"os"
	"io/ioutil"
	"net"
	"net/http"

	"k8s.io/client-go/rest"
	
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/internal"
	"sigs.k8s.io/controller-runtime/pkg/envtest/setup/addr"
)

// NewClientCertAuth returns a client-certs-based UserProvisioner.
func NewClientCertAuth(baseDir string) (UserProvisioner, error) {
	ca, err := internal.NewTinyCA()
	if err != nil {
		return nil, fmt.Errorf("unable to set up CA: %v", err)
	}
	ca.BaseDir = baseDir

	return ca, nil
}

// User is the result of a Kubernetes token authentication webhook.
// It's identical to the k8s UserInfo object from TokenReview object,
// but re-exported here to make API compat easier.
type User = internal.User

// TokenLookup abstracts over a Kubernetes token authn webhook.
type TokenLookup = internal.TokenLookup

// Token is a kubernetes authentication token.
type Token = internal.Token

// TokenList implements a TokenLookup based on a map of tokens to user info.
type TokenList struct {
	// users maps known tokens to users.
	users map[Token]User
	// audiences maps known tokens to allowed audiences.
	audiences map[Token][]string

	// nextToken holds the index for the next auto-generated token
	// to be produced by NewUser.
	nextToken int

	mu sync.RWMutex
}

// NewTokenList produces a new empty TokenList.
func NewTokenList() *TokenList {
	return &TokenList{
		users: make(map[Token]User),
		audiences: make(map[Token][]string),
	}
}

// AddToken maps the given token to the given user with the given audiences.
func (t *TokenList) AddToken(token Token, info User, audiences ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.users[token] = info
	t.audiences[token] = audiences
}

// RemoveToken removes the given token from the list of known tokens.
func (t *TokenList) RemoveToken(token Token) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.users, token)
}

// NewUser auto-provisions a new token, then maps it to the given user with the given audiences.
func (t *TokenList) NewUser(info User, audiences ...string) Token { 
	t.mu.Lock()
	defer t.mu.Unlock()

	tokInd := t.nextToken
	t.nextToken++

	tok := Token(fmt.Sprintf("generated-%d", tokInd))

	// don't call AddUser b/c that holds the lock
	t.users[tok] = info
	t.audiences[tok] = audiences
	
	return tok
}

func (t *TokenList) Check(token Token) (info *User, allowedAudiences []string, err error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	outInfo, known := t.users[token]
	if !known {
		// TODO(directxman12): the docs gesticulate wildly and say that http
		// error codes may be used, but not which ones are expected.  Figure
		// this out by traipsing through the source.
		return nil, nil, nil
	}

	return &outInfo, t.audiences[token], nil
}

// TokenProvisioner turns a TokenLookup into a UserProvisioner.
type TokenProvisioner struct {
	// Lookup is the TokenLookup used to verify tokens.
	Lookup TokenLookup

	// configFileName is the name of the file to used to store
	// the token configuration.  It'll get autoset to a temporary
	// file.
	configFileName string

	// server serves the webhook
	server *http.Server
}

func (h *TokenProvisioner) Start() ([]string, error) {
	cfgFile, err := ioutil.TempFile("", "testenv-webhook-config-*.yaml")
	defer cfgFile.Close()

	h.configFileName = cfgFile.Name()

	port, host, err := addr.Suggest()
	if err != nil {
		return nil, err
	}
	
	_, err = fmt.Fprintf(cfgFile, `
apiVersion: v1
kind: Config
clusters:
- name: local-auth-hook
  cluster:
    server: http://%s:%d/
users:
- name: envtest-apiserver
  # TODO: client-cert
contexts:
- name: webhook
  context:
    cluster: local-auth-hook
    user: envtest-apiserver
current-context: webhook
`, host, port)
	if err != nil {
		return nil, err
	}

	h.server = &http.Server{
		Addr: net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		Handler: &internal.TokenHook{Lookup: h.Lookup},
	}
	go h.server.ListenAndServe()

	// TODO(directxman12): make this a template or something
	return []string{"--authentication-token-webhook-config-file="+h.configFileName}, nil
}

func (h *TokenProvisioner) Stop() error {
	if h.configFileName != "" {
		os.Remove(h.configFileName)
	}

	if err := h.server.Close(); err != nil {
		return err
	}

	return nil
}

func (h *TokenProvisioner) RegisterUser(info User, cfg *rest.Config) error {
	token := h.Lookup.NewUser(info)
	cfg.BearerToken = string(token)
	return nil
}
