// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 The Kubernetes Authors

package workflows_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5" //nolint:gosec
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
)

var (
	remoteNamesGCS = []string{
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
	remoteVersionsGCS = makeContentsGCS(remoteNamesGCS)

	remoteNamesHTTP = remote.Index{
		Releases: map[string]remote.Release{
			"v1.10.0": map[string]remote.Archive{
				"envtest-v1.10-darwin-amd64.tar.gz": {},
				"envtest-v1.10-linux-amd64.tar.gz":  {},
			},
			"v1.10.1": map[string]remote.Archive{
				"envtest-v1.10.1-darwin-amd64.tar.gz": {},
				"envtest-v1.10.1-linux-amd64.tar.gz":  {},
			},
			"v1.11.0": map[string]remote.Archive{
				"envtest-v1.11.0-darwin-amd64.tar.gz": {},
				"envtest-v1.11.0-linux-amd64.tar.gz":  {},
			},
			"v1.11.1": map[string]remote.Archive{
				"envtest-v1.11.1-potato-cherrypie.tar.gz": {},
			},
			"v1.12.3": map[string]remote.Archive{
				"envtest-v1.12.3-darwin-amd64.tar.gz": {},
				"envtest-v1.12.3-linux-amd64.tar.gz":  {},
			},
			"v1.13.1": map[string]remote.Archive{
				"envtest-v1.13.1-darwin-amd64.tar.gz": {},
				"envtest-v1.13.1-linux-amd64.tar.gz":  {},
			},
			"v1.14.1": map[string]remote.Archive{
				"envtest-v1.14.1-darwin-amd64.tar.gz": {},
				"envtest-v1.14.1-linux-amd64.tar.gz":  {},
			},
			"v1.15.5": map[string]remote.Archive{
				"envtest-v1.15.5-darwin-amd64.tar.gz": {},
				"envtest-v1.15.5-linux-amd64.tar.gz":  {},
			},
			"v1.16.4": map[string]remote.Archive{
				"envtest-v1.16.4-darwin-amd64.tar.gz": {},
				"envtest-v1.16.4-linux-amd64.tar.gz":  {},
			},
			"v1.17.9": map[string]remote.Archive{
				"envtest-v1.17.9-darwin-amd64.tar.gz": {},
				"envtest-v1.17.9-linux-amd64.tar.gz":  {},
			},
			"v1.19.0": map[string]remote.Archive{
				"envtest-v1.19.0-darwin-amd64.tar.gz": {},
				"envtest-v1.19.0-linux-amd64.tar.gz":  {},
			},
			"v1.19.2": map[string]remote.Archive{
				"envtest-v1.19.2-darwin-amd64.tar.gz":  {},
				"envtest-v1.19.2-linux-amd64.tar.gz":   {},
				"envtest-v1.19.2-linux-arm64.tar.gz":   {},
				"envtest-v1.19.2-linux-ppc64le.tar.gz": {},
			},
			"v1.20.2": map[string]remote.Archive{
				"envtest-v1.20.2-darwin-amd64.tar.gz":  {},
				"envtest-v1.20.2-linux-amd64.tar.gz":   {},
				"envtest-v1.20.2-linux-arm64.tar.gz":   {},
				"envtest-v1.20.2-linux-ppc64le.tar.gz": {},
			},
		},
	}
	remoteVersionsHTTP = makeContentsHTTP(remoteNamesHTTP)

	// keep this sorted.
	localVersions = []versions.Set{
		{Version: ver(1, 17, 9), Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
		}},
		{Version: ver(1, 16, 2), Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "yourimagination"}},
			{Platform: versions.Platform{OS: "ifonlysingularitywasstillathing", Arch: "amd64"}},
		}},
		{Version: ver(1, 16, 1), Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
		}},
		{Version: ver(1, 16, 0), Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
		}},
		{Version: ver(1, 14, 26), Platforms: []versions.PlatformItem{
			{Platform: versions.Platform{OS: "linux", Arch: "amd64"}},
			{Platform: versions.Platform{OS: "hyperwarp", Arch: "pixiedust"}},
		}},
	}
)

type item struct {
	meta     bucketObject
	contents []byte
}

// objectList is the parts we need of the GCS "list-objects-in-bucket" endpoint.
type objectList struct {
	Items []bucketObject `json:"items"`
}

// bucketObject is the parts we need of the GCS object metadata.
type bucketObject struct {
	Name string `json:"name"`
	Hash string `json:"md5Hash"`
}

func makeContentsGCS(names []string) []item {
	res := make([]item, len(names))
	for i, name := range names {
		var chunk [1024 * 48]byte // 1.5 times our chunk read size in GetVersion
		copy(chunk[:], name)
		if _, err := rand.Read(chunk[len(name):]); err != nil {
			panic(err)
		}
		res[i] = verWithGCS(name, chunk[:])
	}
	return res
}

func verWithGCS(name string, contents []byte) item {
	out := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(out)
	tarWriter := tar.NewWriter(gzipWriter)
	err := tarWriter.WriteHeader(&tar.Header{
		Name: "kubebuilder/bin/some-file",
		Size: int64(len(contents)),
		Mode: 0777, // so we can check that we fix this later
	})
	if err != nil {
		panic(err)
	}
	_, err = tarWriter.Write(contents)
	if err != nil {
		panic(err)
	}
	tarWriter.Close()
	gzipWriter.Close()
	res := item{
		meta:     bucketObject{Name: name},
		contents: out.Bytes(),
	}
	hash := md5.Sum(res.contents) //nolint:gosec
	res.meta.Hash = base64.StdEncoding.EncodeToString(hash[:])
	return res
}

func handleRemoteVersionsGCS(server *ghttp.Server, versions []item) {
	list := objectList{Items: make([]bucketObject, len(versions))}
	for i, ver := range versions {
		ver := ver // copy to avoid capturing the iteration variable
		list.Items[i] = ver.meta
		server.RouteToHandler("GET", "/storage/v1/b/kubebuilder-tools-test/o/"+ver.meta.Name, func(resp http.ResponseWriter, req *http.Request) {
			if req.URL.Query().Get("alt") == "media" {
				resp.WriteHeader(http.StatusOK)
				Expect(resp.Write(ver.contents)).To(Equal(len(ver.contents)))
			} else {
				ghttp.RespondWithJSONEncoded(
					http.StatusOK,
					ver.meta,
				)(resp, req)
			}
		})
	}
	server.RouteToHandler("GET", "/storage/v1/b/kubebuilder-tools-test/o", ghttp.RespondWithJSONEncoded(
		http.StatusOK,
		list,
	))
}

type itemsHTTP struct {
	index    remote.Index
	contents map[string][]byte
}

func makeContentsHTTP(index remote.Index) itemsHTTP {
	// This creates a new copy of the index so modifying the index
	// in some tests doesn't affect others.
	res := itemsHTTP{
		index: remote.Index{
			Releases: map[string]remote.Release{},
		},
		contents: map[string][]byte{},
	}

	for releaseVersion, releases := range index.Releases {
		res.index.Releases[releaseVersion] = remote.Release{}
		for archiveName := range releases {
			var chunk [1024 * 48]byte // 1.5 times our chunk read size in GetVersion
			copy(chunk[:], archiveName)
			if _, err := rand.Read(chunk[len(archiveName):]); err != nil {
				panic(err)
			}
			content, hash := verWithHTTP(chunk[:])

			res.index.Releases[releaseVersion][archiveName] = remote.Archive{
				Hash: hash,
				// Note: Only storing the name of the archive for now.
				// This will be expanded later to a full URL once the server is running.
				SelfLink: archiveName,
			}
			res.contents[archiveName] = content
		}
	}
	return res
}

func verWithHTTP(contents []byte) ([]byte, string) {
	out := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(out)
	tarWriter := tar.NewWriter(gzipWriter)
	err := tarWriter.WriteHeader(&tar.Header{
		Name: "controller-tools/envtest/some-file",
		Size: int64(len(contents)),
		Mode: 0777, // so we can check that we fix this later
	})
	if err != nil {
		panic(err)
	}
	_, err = tarWriter.Write(contents)
	if err != nil {
		panic(err)
	}
	tarWriter.Close()
	gzipWriter.Close()
	content := out.Bytes()
	// controller-tools is using sha512
	hash := sha512.Sum512(content)
	hashEncoded := hex.EncodeToString(hash[:])
	return content, hashEncoded
}

func handleRemoteVersionsHTTP(server *ghttp.Server, items itemsHTTP) {
	if server.HTTPTestServer == nil {
		// Just return for test cases where server is closed in BeforeEach. Otherwise server.Addr() below panics.
		return
	}

	// The index from items contains only relative SelfLinks.
	// finalIndex will contain the full links based on server.Addr().
	finalIndex := remote.Index{
		Releases: map[string]remote.Release{},
	}

	for releaseVersion, releases := range items.index.Releases {
		finalIndex.Releases[releaseVersion] = remote.Release{}

		for archiveName, archive := range releases {
			finalIndex.Releases[releaseVersion][archiveName] = remote.Archive{
				Hash:     archive.Hash,
				SelfLink: fmt.Sprintf("http://%s/%s", server.Addr(), archive.SelfLink),
			}
			content := items.contents[archiveName]

			// Note: Using the relative path from archive here instead of the full path.
			server.RouteToHandler("GET", "/"+archive.SelfLink, func(resp http.ResponseWriter, req *http.Request) {
				resp.WriteHeader(http.StatusOK)
				Expect(resp.Write(content)).To(Equal(len(content)))
			})
		}
	}

	indexYAML, err := yaml.Marshal(finalIndex)
	Expect(err).ToNot(HaveOccurred())

	server.RouteToHandler("GET", "/envtest-releases.yaml", ghttp.RespondWith(
		http.StatusOK,
		indexYAML,
	))
}

func fakeStore(fs afero.Afero, dir string) {
	By("making the unpacked directory")
	unpackedBase := filepath.Join(dir, "k8s")
	Expect(fs.Mkdir(unpackedBase, 0755)).To(Succeed())

	By("making some fake (empty) versions")
	for _, set := range localVersions {
		for _, plat := range set.Platforms {
			Expect(fs.Mkdir(filepath.Join(unpackedBase, plat.BaseName(set.Version)), 0755)).To(Succeed())
		}
	}

	By("making some fake non-store paths")
	Expect(fs.Mkdir(filepath.Join(dir, "missing-binaries"), 0755)).To(Succeed())

	Expect(fs.Mkdir(filepath.Join(dir, "wrong-version"), 0755)).To(Succeed())
	Expect(fs.WriteFile(filepath.Join(dir, "wrong-version", "kube-apiserver"), nil, 0755)).To(Succeed())
	Expect(fs.WriteFile(filepath.Join(dir, "wrong-version", "kubectl"), nil, 0755)).To(Succeed())
	Expect(fs.WriteFile(filepath.Join(dir, "wrong-version", "etcd"), nil, 0755)).To(Succeed())

	Expect(fs.Mkdir(filepath.Join(dir, "good-version"), 0755)).To(Succeed())
	Expect(fs.WriteFile(filepath.Join(dir, "good-version", "kube-apiserver"), nil, 0755)).To(Succeed())
	Expect(fs.WriteFile(filepath.Join(dir, "good-version", "kubectl"), nil, 0755)).To(Succeed())
	Expect(fs.WriteFile(filepath.Join(dir, "good-version", "etcd"), nil, 0755)).To(Succeed())
	// TODO: put the right files
}
