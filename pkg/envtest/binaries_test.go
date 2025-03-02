/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package envtest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Test download binaries", func() {
	var downloadDirectory string
	var server *ghttp.Server

	BeforeEach(func() {
		downloadDirectory = GinkgoT().TempDir()

		server = ghttp.NewServer()
		DeferCleanup(func() {
			server.Close()
		})
		setupServer(server)
	})

	It("should download binaries of latest stable version", func() {
		apiServerPath, etcdPath, kubectlPath, err := downloadBinaryAssets(context.Background(), downloadDirectory, "", fmt.Sprintf("http://%s/%s", server.Addr(), "envtest-releases.yaml"))
		Expect(err).ToNot(HaveOccurred())

		// Verify latest stable version (v1.32.0) was downloaded
		versionDownloadDirectory := path.Join(downloadDirectory, fmt.Sprintf("1.32.0-%s-%s", runtime.GOOS, runtime.GOARCH))
		Expect(apiServerPath).To(Equal(path.Join(versionDownloadDirectory, "kube-apiserver")))
		Expect(etcdPath).To(Equal(path.Join(versionDownloadDirectory, "etcd")))
		Expect(kubectlPath).To(Equal(path.Join(versionDownloadDirectory, "kubectl")))

		dirEntries, err := os.ReadDir(versionDownloadDirectory)
		Expect(err).ToNot(HaveOccurred())
		var actualFiles []string
		for _, e := range dirEntries {
			actualFiles = append(actualFiles, e.Name())
		}
		Expect(actualFiles).To(ConsistOf("some-file"))
	})

	It("should download v1.32.0 binaries", func() {
		apiServerPath, etcdPath, kubectlPath, err := downloadBinaryAssets(context.Background(), downloadDirectory, "v1.31.0", fmt.Sprintf("http://%s/%s", server.Addr(), "envtest-releases.yaml"))
		Expect(err).ToNot(HaveOccurred())

		// Verify latest stable version (v1.32.0) was downloaded
		versionDownloadDirectory := path.Join(downloadDirectory, fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH))
		Expect(apiServerPath).To(Equal(path.Join(versionDownloadDirectory, "kube-apiserver")))
		Expect(etcdPath).To(Equal(path.Join(versionDownloadDirectory, "etcd")))
		Expect(kubectlPath).To(Equal(path.Join(versionDownloadDirectory, "kubectl")))

		dirEntries, err := os.ReadDir(versionDownloadDirectory)
		Expect(err).ToNot(HaveOccurred())
		var actualFiles []string
		for _, e := range dirEntries {
			actualFiles = append(actualFiles, e.Name())
		}
		Expect(actualFiles).To(ConsistOf("some-file"))
	})
})

var (
	envtestBinaryArchives = index{
		Releases: map[string]release{
			"v1.32.0": map[string]archive{
				"envtest-v1.32.0-darwin-amd64.tar.gz":  {},
				"envtest-v1.32.0-darwin-arm64.tar.gz":  {},
				"envtest-v1.32.0-linux-amd64.tar.gz":   {},
				"envtest-v1.32.0-linux-arm64.tar.gz":   {},
				"envtest-v1.32.0-linux-ppc64le.tar.gz": {},
				"envtest-v1.32.0-linux-s390x.tar.gz":   {},
				"envtest-v1.32.0-windows-amd64.tar.gz": {},
			},
			"v1.31.0": map[string]archive{
				"envtest-v1.31.0-darwin-amd64.tar.gz":  {},
				"envtest-v1.31.0-darwin-arm64.tar.gz":  {},
				"envtest-v1.31.0-linux-amd64.tar.gz":   {},
				"envtest-v1.31.0-linux-arm64.tar.gz":   {},
				"envtest-v1.31.0-linux-ppc64le.tar.gz": {},
				"envtest-v1.31.0-linux-s390x.tar.gz":   {},
				"envtest-v1.31.0-windows-amd64.tar.gz": {},
			},
		},
	}
)

func setupServer(server *ghttp.Server) {
	itemsHTTP := makeArchives(envtestBinaryArchives)

	// The index from itemsHTTP contains only relative SelfLinks.
	// finalIndex will contain the full links based on server.Addr().
	finalIndex := index{
		Releases: map[string]release{},
	}

	for releaseVersion, releases := range itemsHTTP.index.Releases {
		finalIndex.Releases[releaseVersion] = release{}

		for archiveName, a := range releases {
			finalIndex.Releases[releaseVersion][archiveName] = archive{
				Hash:     a.Hash,
				SelfLink: fmt.Sprintf("http://%s/%s", server.Addr(), a.SelfLink),
			}
			content := itemsHTTP.contents[archiveName]

			// Note: Using the relative path from archive here instead of the full path.
			server.RouteToHandler("GET", "/"+a.SelfLink, func(resp http.ResponseWriter, req *http.Request) {
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

type itemsHTTP struct {
	index    index
	contents map[string][]byte
}

func makeArchives(i index) itemsHTTP {
	// This creates a new copy of the index so modifying the index
	// in some tests doesn't affect others.
	res := itemsHTTP{
		index: index{
			Releases: map[string]release{},
		},
		contents: map[string][]byte{},
	}

	for releaseVersion, releases := range i.Releases {
		res.index.Releases[releaseVersion] = release{}
		for archiveName := range releases {
			var chunk [1024 * 48]byte // 1.5 times our chunk read size in GetVersion
			copy(chunk[:], archiveName)
			if _, err := rand.Read(chunk[len(archiveName):]); err != nil {
				panic(err)
			}
			content, hash := makeArchive(chunk[:])

			res.index.Releases[releaseVersion][archiveName] = archive{
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

func makeArchive(contents []byte) ([]byte, string) {
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
