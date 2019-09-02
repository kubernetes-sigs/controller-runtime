package leaderelection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	tlog "github.com/go-logr/logr/testing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/internal/recorder"
)

var _ = Describe("Leader Election", func() {
	It("should use the Lease lock because coordination group is available.", func() {
		coordinationGroup := &v1.APIGroupList{
			Groups: []v1.APIGroup{
				{Name: coordinationv1.GroupName},
			},
		}

		clientConfig := &restclient.Config{
			Transport: interceptAPIGroupCall(coordinationGroup),
		}

		rProvider, err := recorder.NewProvider(clientConfig, scheme.Scheme, tlog.NullLogger{}, record.NewBroadcaster())
		Expect(err).ToNot(HaveOccurred())

		lock, err := NewResourceLock(clientConfig, rProvider, Options{LeaderElection: true, LeaderElectionNamespace: "test-ns"})
		Expect(err).ToNot(HaveOccurred())
		Expect(lock).To(BeAssignableToTypeOf(&resourcelock.LeaseLock{}))
	})

	It("should use the ConfigMap lock because coordination group is unavailable.", func() {
		clientConfig := &restclient.Config{
			Transport: interceptAPIGroupCall(&v1.APIGroupList{ /* no coordination group */ }),
		}

		rProvider, err := recorder.NewProvider(clientConfig, scheme.Scheme, tlog.NullLogger{}, record.NewBroadcaster())
		Expect(err).ToNot(HaveOccurred())

		lock, err := NewResourceLock(clientConfig, rProvider, Options{LeaderElection: true, LeaderElectionNamespace: "test-ns"})
		Expect(err).ToNot(HaveOccurred())
		Expect(lock).To(BeAssignableToTypeOf(&resourcelock.ConfigMapLock{}))
	})
})

func interceptAPIGroupCall(returnApis *v1.APIGroupList) roundTripper {
	return roundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method == "GET" && (req.URL.Path == "/apis" || req.URL.Path == "/api") {
			return encode(returnApis)
		}
		return nil, fmt.Errorf("unexpected request: %v %#v\n%#v", req.Method, req.URL, req)
	})
}
func encode(bodyStruct interface{}) (*http.Response, error) {
	jsonBytes, err := json.Marshal(bodyStruct)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(bytes.NewReader(jsonBytes)),
	}, nil
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
