package internal

import (
	"net/http"
	"encoding/json"
	
	authapi "k8s.io/api/authentication/v1beta1"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var hookLog = logf.Log.WithName("testenv-token-webhook")

// Token is a kubernetes authentication token.
type Token string

// User is the result of a Kubernetes token authentication webhook.
// It's identical to the k8s UserInfo object from TokenReview object,
// but re-exported here to make API compat easier.
type User authapi.UserInfo

// TokenLookup abstracts over a Kubernetes token authn webhook.
type TokenLookup interface {
	// Check checks if the given token is allowed.  If it is, user info will be returned.
	// If not, no info is returned, but an error may be.
	Check(token Token) (info *User, allowedAudiences []string, err error)

	// NewUser auto-provisions a new token, then maps it to the given user with the given audiences.
	NewUser(info User, audiences ...string) Token
}

// TokenHook serves an kubernetes token authn webhook based on a TokenLookup.
// This can be used to simulate different authentication of different users to
// the API server.
type TokenHook struct {
	// Lookup is used to check the validity and details of the given token.
	Lookup TokenLookup
}

func (h *TokenHook) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// TODO(directxman12): logging in here

	var rev authapi.TokenReview
	if err := json.NewDecoder(req.Body).Decode(&rev); err != nil {
		hookLog.Error(err, "unable to decode request body")
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	if rev.APIVersion != "authentication.k8s.io/v1beta1" || rev.Kind != "TokenReview" {
		hookLog.Error(nil, "bad api version or kind", "api version", rev.APIVersion, "kind", rev.Kind)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	maybeInfo, audiences, err := h.Lookup.Check(Token(rev.Spec.Token))
	if err != nil {
		hookLog.Error(err, "unable to check token")
		// TODO(directxman12): return a valid response with the error field set
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	if maybeInfo == nil {
		rev.Status = authapi.TokenReviewStatus{Authenticated: false}
	} else {
		rev.Status = authapi.TokenReviewStatus{
			Authenticated: true,
			User: authapi.UserInfo(*maybeInfo),
			Audiences: audiences,
		}
	}

	resp.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(resp).Encode(&rev); err != nil {
		hookLog.Error(err, "unable to encode response")
	}
}
