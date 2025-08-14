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
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"sigs.k8s.io/yaml"
)

func TestInterpretKubernetesVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		in    []string
		exact bool
		major uint
		minor uint
	}{
		{
			name:  `SemVer and "v" prefix are exact`,
			exact: true,
			in: []string{
				"1.2.3", "v1.2.3", "v1.30.2", "v1.31.0-beta.0", "v1.33.0-alpha.2",
			},
		},
		{
			name: "leading zeroes are not a version",
			in: []string{
				"01.2.0", "00001.2.3", "1.2.03", "v01.02.0003",
			},
		},
		{
			name: "weird stuff is not a version",
			in: []string{
				"asdf", "version", "vegeta4", "the.1", "2ne1", "=7.8.9", "10.x", "*",
				"0.0001", "1.00002", "v1.2anything", "1.2.x", "1.2.z", "1.2.*",
			},
		},
		{
			name: "one number is not a version",
			in: []string{
				"1", "v1", "v001", "1.", "v1.", "1.x",
			},
		},
		{
			name:  "two numbers are a release series",
			major: 0, minor: 1,
			in: []string{"0.1", "v0.1"},
		},
		{
			name:  "two numbers are a release series",
			major: 1, minor: 2,
			in: []string{"1.2", "v1.2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, input := range tc.in {
				exact, major, minor := interpretKubernetesVersion(input)

				if tc.exact {
					if expected := strings.TrimPrefix(input, "v"); exact != expected {
						t.Errorf("expected canonical %q for %q, got %q", expected, input, exact)
					}
					if major != 0 || minor != 0 {
						t.Errorf("expected no release series for %q, got (%v, %v)", input, major, minor)
					}
				}

				if !tc.exact {
					if major != tc.major {
						t.Errorf("expected major %v for %q, got %v", tc.major, input, major)
					}
					if minor != tc.minor {
						t.Errorf("expected minor %v for %q, got %v", tc.minor, input, minor)
					}
					if exact != "" {
						t.Errorf("expected no canonical version for %q, got %q", input, exact)
					}
				}
			}
		})
	}
}

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

	It("should download binaries of latest stable version", func(ctx SpecContext) {
		apiServerPath, etcdPath, kubectlPath, err := downloadBinaryAssets(ctx, downloadDirectory, "", fmt.Sprintf("http://%s/%s", server.Addr(), "envtest-releases.yaml"))
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

	It("should download binaries of an exact version", func(ctx SpecContext) {
		apiServerPath, etcdPath, kubectlPath, err := downloadBinaryAssets(ctx, downloadDirectory, "v1.31.0", fmt.Sprintf("http://%s/%s", server.Addr(), "envtest-releases.yaml"))
		Expect(err).ToNot(HaveOccurred())

		// Verify exact version (v1.31.0) was downloaded
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

	It("should download binaries of latest stable version of a release series", func(ctx SpecContext) {
		apiServerPath, etcdPath, kubectlPath, err := downloadBinaryAssets(ctx, downloadDirectory, "1.31", fmt.Sprintf("http://%s/%s", server.Addr(), "envtest-releases.yaml"))
		Expect(err).ToNot(HaveOccurred())

		// Verify stable version (v1.31.4) was downloaded
		versionDownloadDirectory := path.Join(downloadDirectory, fmt.Sprintf("1.31.4-%s-%s", runtime.GOOS, runtime.GOARCH))
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
			"v1.31.4": map[string]archive{
				"envtest-v1.31.4-darwin-amd64.tar.gz":  {},
				"envtest-v1.31.4-darwin-arm64.tar.gz":  {},
				"envtest-v1.31.4-linux-amd64.tar.gz":   {},
				"envtest-v1.31.4-linux-arm64.tar.gz":   {},
				"envtest-v1.31.4-linux-ppc64le.tar.gz": {},
				"envtest-v1.31.4-linux-s390x.tar.gz":   {},
				"envtest-v1.31.4-windows-amd64.tar.gz": {},
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
