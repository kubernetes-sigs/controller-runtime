package testhelpers

import (
	"errors"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

// objectList is the parts we need of the GCS "list-objects-in-bucket" endpoint.
type objectList struct {
	Items []BucketObject `json:"items"`
}

// BucketObject is the parts we need of the GCS object metadata.
type BucketObject struct {
	Name string `json:"name"`
	Hash string `json:"md5Hash"`
}

// Item represents a single object in the mock server.
type Item struct {
	Meta     BucketObject
	Contents []byte
}

var (
	// RemoteNames are all the package names that can be used on the mock server.
	// Provide this, or a subset of it, to NewServerWithContents to get a mock server that knows about those packages.
	RemoteNames = []string{
		"kubebuilder-tools-1.10-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.10-linux-amd64.tar.gz",
		"kubebuilder-tools-1.10.1-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.10.1-linux-amd64.tar.gz",
		"kubebuilder-tools-1.11.0-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.11.0-linux-amd64.tar.gz",
		"kubebuilder-tools-1.11.1-potato-cherrypie.tar.gz",
		"kubebuilder-tools-1.12.3-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.12.3-linux-amd64.tar.gz",
		"kubebuilder-tools-1.13.1-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.13.1-linux-amd64.tar.gz",
		"kubebuilder-tools-1.14.1-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.14.1-linux-amd64.tar.gz",
		"kubebuilder-tools-1.15.5-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.15.5-linux-amd64.tar.gz",
		"kubebuilder-tools-1.16.4-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.16.4-linux-amd64.tar.gz",
		"kubebuilder-tools-1.17.9-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.17.9-linux-amd64.tar.gz",
		"kubebuilder-tools-1.19.0-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.19.0-linux-amd64.tar.gz",
		"kubebuilder-tools-1.19.2-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.19.2-linux-amd64.tar.gz",
		"kubebuilder-tools-1.19.2-linux-arm64.tar.gz",
		"kubebuilder-tools-1.19.2-linux-ppc64le.tar.gz",
		"kubebuilder-tools-1.20.2-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.20.2-linux-amd64.tar.gz",
		"kubebuilder-tools-1.20.2-linux-arm64.tar.gz",
		"kubebuilder-tools-1.20.2-linux-ppc64le.tar.gz",
		"kubebuilder-tools-1.9-darwin-amd64.tar.gz",
		"kubebuilder-tools-1.9-linux-amd64.tar.gz",
		"kubebuilder-tools-v1.19.2-darwin-amd64.tar.gz",
		"kubebuilder-tools-v1.19.2-linux-amd64.tar.gz",
		"kubebuilder-tools-v1.19.2-linux-arm64.tar.gz",
		"kubebuilder-tools-v1.19.2-linux-ppc64le.tar.gz",
	}

	contents map[string]Item
)

func makeContents(names []string) ([]Item, error) {
	res := make([]Item, len(names))
	if contents == nil {
		contents = make(map[string]Item, len(RemoteNames))
	}

	var errs error
	for i, name := range names {
		if item, ok := contents[name]; ok {
			res[i] = item
			continue
		}

		chunk, err := ContentsFor(name)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}

		item := verWith(name, chunk)
		contents[name] = item
		res[i] = item
	}

	if errs != nil {
		return nil, errs
	}

	return res, nil
}

// NewServer spins up a mock server that knows about the provided packages.
// The package names should be a subset of RemoteNames.
//
// The returned shutdown function should be called at the end of the test
func NewServer(items ...Item) (addr string, shutdown func(), err error) {
	if items == nil {
		versions, err := makeContents(RemoteNames)
		if err != nil {
			return "", nil, err
		}
		items = versions
	}

	server := ghttp.NewServer()

	list := objectList{Items: make([]BucketObject, len(items))}
	for i, ver := range items {
		ver := ver // copy to avoid capturing the iteration variable
		list.Items[i] = ver.Meta
		server.RouteToHandler("GET", "/storage/v1/b/kubebuilder-tools-test/o/"+ver.Meta.Name, func(resp http.ResponseWriter, req *http.Request) {
			if req.URL.Query().Get("alt") == "media" {
				resp.WriteHeader(http.StatusOK)
				gomega.Expect(resp.Write(ver.Contents)).To(gomega.Equal(len(ver.Contents)))
			} else {
				ghttp.RespondWithJSONEncoded(
					http.StatusOK,
					ver.Meta,
				)(resp, req)
			}
		})
	}
	server.RouteToHandler("GET", "/storage/v1/b/kubebuilder-tools-test/o", ghttp.RespondWithJSONEncoded(
		http.StatusOK,
		list,
	))

	return server.Addr(), server.Close, nil
}
