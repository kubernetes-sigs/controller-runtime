// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 The Kubernetes Authors

package workflows_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/util/sets"
	envp "sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/store"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
	wf "sigs.k8s.io/controller-runtime/tools/setup-envtest/workflows"
)

func ver(major, minor, patch int) versions.Concrete {
	return versions.Concrete{
		Major: major,
		Minor: minor,
		Patch: patch,
	}
}

func shouldHaveError() {
	var err error
	var code int
	if cause := recover(); envp.CheckRecover(cause, func(caughtCode int, caughtErr error) {
		err = caughtErr
		code = caughtCode
	}) {
		panic(cause)
	}
	Expect(err).To(HaveOccurred(), "should write an error")
	Expect(code).NotTo(BeZero(), "should exit with a non-zero code")
}

const (
	testStorePath = ".teststore"
)

const (
	gcsMode  = "GCS"
	httpMode = "HTTP"
)

var _ = Describe("GCS Client", func() {
	WorkflowTest(gcsMode)
})

var _ = Describe("HTTP Client", func() {
	WorkflowTest(httpMode)
})

func WorkflowTest(testMode string) {
	Describe("Workflows", func() {
		var (
			env             *envp.Env
			out             *bytes.Buffer
			server          *ghttp.Server
			remoteGCSItems  []item
			remoteHTTPItems itemsHTTP
		)
		BeforeEach(func() {
			out = new(bytes.Buffer)
			baseFs := afero.Afero{Fs: afero.NewMemMapFs()}

			server = ghttp.NewServer()

			var client remote.Client
			switch testMode {
			case gcsMode:
				client = &remote.GCSClient{ //nolint:staticcheck // deprecation accepted for now
					Log:      testLog.WithName("gcs-client"),
					Bucket:   "kubebuilder-tools-test", // test custom bucket functionality too
					Server:   server.Addr(),
					Insecure: true, // no https in httptest :-(
				}
			case httpMode:
				client = &remote.HTTPClient{
					Log:      testLog.WithName("http-client"),
					IndexURL: fmt.Sprintf("http://%s/%s", server.Addr(), "envtest-releases.yaml"),
				}
			}

			env = &envp.Env{
				Log:       testLog,
				VerifySum: true, // on by default
				FS:        baseFs,
				Store:     &store.Store{Root: afero.NewBasePathFs(baseFs, testStorePath)},
				Out:       out,
				Platform: versions.PlatformItem{ // default
					Platform: versions.Platform{
						OS:   "linux",
						Arch: "amd64",
					},
				},
				Client: client,
			}

			fakeStore(env.FS, testStorePath)
			remoteGCSItems = remoteVersionsGCS
			remoteHTTPItems = remoteVersionsHTTP
		})
		JustBeforeEach(func() {
			switch testMode {
			case gcsMode:
				handleRemoteVersionsGCS(server, remoteGCSItems)
			case httpMode:
				handleRemoteVersionsHTTP(server, remoteHTTPItems)
			}
		})
		AfterEach(func() {
			server.Close()
			server = nil
		})

		Describe("use", func() {
			var flow wf.Use
			BeforeEach(func() {
				// some defaults for most tests
				env.Version = versions.Spec{
					Selector: ver(1, 16, 0),
				}
				flow = wf.Use{
					PrintFormat: envp.PrintPath,
				}
			})

			It("should initialize the store if it doesn't exist", func() {
				Expect(env.FS.RemoveAll(testStorePath)).To(Succeed())
				// need to set this to a valid remote version cause our store is now empty
				env.Version = versions.Spec{Selector: ver(1, 16, 4)}
				flow.Do(env)
				Expect(env.FS.Stat(testStorePath)).NotTo(BeNil())
			})

			Context("when use env is set", func() {
				BeforeEach(func() {
					flow.UseEnv = true
				})
				It("should fall back to normal behavior when the env is not set", func() {
					flow.Do(env)
					Expect(out.String()).To(HaveSuffix("/1.16.0-linux-amd64"), "should fall back to a local version")
				})
				It("should fall back to normal behavior if binaries are missing", func() {
					flow.AssetsPath = ".teststore/missing-binaries"
					flow.Do(env)
					Expect(out.String()).To(HaveSuffix("/1.16.0-linux-amd64"), "should fall back to a local version")
				})
				It("should use the value of the env if it contains the right binaries", func() {
					flow.AssetsPath = ".teststore/good-version"
					flow.Do(env)
					Expect(out.String()).To(Equal(flow.AssetsPath))
				})
				It("should not try and check the version of the binaries", func() {
					flow.AssetsPath = ".teststore/wrong-version"
					flow.Do(env)
					Expect(out.String()).To(Equal(flow.AssetsPath))
				})
				It("should not need to contact the network", func() {
					server.Close()
					flow.AssetsPath = ".teststore/good-version"
					flow.Do(env)
					// expect to not get a panic -- if we do, it'll cause the test to fail
				})
			})

			Context("when downloads are disabled", func() {
				BeforeEach(func() {
					env.NoDownload = true
					server.Close()
				})

				// It("should not contact the network") is a gimme here, because we
				// call server.Close() above.

				It("should error if no matches are found locally", func() {
					defer shouldHaveError()
					env.Version.Selector = versions.Concrete{Major: 9001}
					flow.Do(env)
				})
				It("should settle for the latest local match if latest is requested", func() {
					env.Version = versions.Spec{
						CheckLatest: true,
						Selector: versions.PatchSelector{
							Major: 1,
							Minor: 16,
							Patch: versions.AnyPoint,
						},
					}

					flow.Do(env)

					// latest on "server" is 1.16.4, shouldn't use that
					Expect(out.String()).To(HaveSuffix("/1.16.1-linux-amd64"), "should use the latest local version")
				})
			})

			Context("if latest is requested", func() {
				It("should contact the network to see if there's anything newer", func() {
					env.Version = versions.Spec{
						CheckLatest: true,
						Selector: versions.PatchSelector{
							Major: 1, Minor: 16, Patch: versions.AnyPoint,
						},
					}
					flow.Do(env)
					Expect(out.String()).To(HaveSuffix("/1.16.4-linux-amd64"), "should use the latest remote version")
				})
				It("should still use the latest local if the network doesn't have anything newer", func() {
					env.Version = versions.Spec{
						CheckLatest: true,
						Selector: versions.PatchSelector{
							Major: 1, Minor: 14, Patch: versions.AnyPoint,
						},
					}

					flow.Do(env)

					// latest on the server is 1.14.1, latest local is 1.14.26
					Expect(out.String()).To(HaveSuffix("/1.14.26-linux-amd64"), "should use the latest local version")
				})
			})

			It("should check local for a match first", func() {
				server.Close() // confirm no network
				env.Version = versions.Spec{
					Selector: versions.TildeSelector{Concrete: ver(1, 16, 0)},
				}
				flow.Do(env)
				// latest on the server is 1.16.4, latest local is 1.16.1
				Expect(out.String()).To(HaveSuffix("/1.16.1-linux-amd64"), "should use the latest local version")
			})

			It("should fall back to the network if no local matches are found", func() {
				env.Version = versions.Spec{
					Selector: versions.TildeSelector{Concrete: ver(1, 19, 0)},
				}
				flow.Do(env)
				Expect(out.String()).To(HaveSuffix("/1.19.2-linux-amd64"), "should have a remote version")
			})

			It("should error out if no matches can be found anywhere", func() {
				defer shouldHaveError()
				env.Version = versions.Spec{
					Selector: versions.TildeSelector{Concrete: ver(0, 0, 1)},
				}
				flow.Do(env)
			})

			It("should skip local versions matches with non-matching platforms", func() {
				env.NoDownload = true // so we get an error
				defer shouldHaveError()
				env.Version = versions.Spec{
					// has non-matching local versions
					Selector: ver(1, 13, 0),
				}

				flow.Do(env)
			})

			It("should skip remote version matches with non-matching platforms", func() {
				defer shouldHaveError()
				env.Version = versions.Spec{
					// has a non-matching remote version
					Selector: versions.TildeSelector{Concrete: ver(1, 11, 1)},
				}
				flow.Do(env)
			})

			Describe("verifying the checksum", func() {
				BeforeEach(func() {
					remoteGCSItems = append(remoteGCSItems, item{
						meta: bucketObject{
							Name: "kubebuilder-tools-86.75.309-linux-amd64.tar.gz",
							Hash: "nottherightone!",
						},
						contents: remoteGCSItems[0].contents, // need a valid tar.gz file to not error from that
					})
					// Recreate remoteHTTPItems to not impact others tests.
					remoteHTTPItems = makeContentsHTTP(remoteNamesHTTP)
					remoteHTTPItems.index.Releases["v86.75.309"] = map[string]remote.Archive{
						"envtest-v86.75.309-linux-amd64.tar.gz": {
							SelfLink: "not used in this test",
							Hash:     "nottherightone!",
						},
					}
					// need a valid tar.gz file to not error from that
					remoteHTTPItems.contents["envtest-v86.75.309-linux-amd64.tar.gz"] = remoteHTTPItems.contents["envtest-v1.10-darwin-amd64.tar.gz"]

					env.Version = versions.Spec{
						Selector: ver(86, 75, 309),
					}
				})
				Specify("when enabled, should fail if the downloaded hash doesn't match", func() {
					defer shouldHaveError()
					flow.Do(env)
				})
				Specify("when disabled, shouldn't check the checksum at all", func() {
					env.VerifySum = false
					flow.Do(env)
				})
			})
		})

		Describe("list", func() {
			// split by fields so we're not matching on whitespace
			listFields := func() [][]string {
				resLines := strings.Split(strings.TrimSpace(out.String()), "\n")
				resFields := make([][]string, len(resLines))
				for i, line := range resLines {
					resFields[i] = strings.Fields(line)
				}
				return resFields
			}

			Context("when downloads are disabled", func() {
				BeforeEach(func() {
					server.Close() // ensure no network
					env.NoDownload = true
				})
				It("should include local contents sorted by version", func() {
					env.Version = versions.AnyVersion
					env.Platform.Platform = versions.Platform{OS: "*", Arch: "*"}
					wf.List{}.Do(env)

					Expect(listFields()).To(Equal([][]string{
						{"(installed)", "v1.17.9", "linux/amd64"},
						{"(installed)", "v1.16.2", "ifonlysingularitywasstillathing/amd64"},
						{"(installed)", "v1.16.2", "linux/yourimagination"},
						{"(installed)", "v1.16.1", "linux/amd64"},
						{"(installed)", "v1.16.0", "linux/amd64"},
						{"(installed)", "v1.14.26", "hyperwarp/pixiedust"},
						{"(installed)", "v1.14.26", "linux/amd64"},
					}))
				})
				It("should skip non-matching local contents", func() {
					env.Version.Selector = versions.PatchSelector{
						Major: 1, Minor: 16, Patch: versions.AnyPoint,
					}
					env.Platform.Arch = "*"
					wf.List{}.Do(env)

					Expect(listFields()).To(Equal([][]string{
						{"(installed)", "v1.16.2", "linux/yourimagination"},
						{"(installed)", "v1.16.1", "linux/amd64"},
						{"(installed)", "v1.16.0", "linux/amd64"},
					}))
				})
			})
			Context("when downloads are enabled", func() {
				Context("when sorting", func() {
					BeforeEach(func() {
						// shorten the list a bit for expediency
						remoteGCSItems = remoteGCSItems[:7]

						// Recreate remoteHTTPItems to not impact others tests.
						remoteHTTPItems = makeContentsHTTP(remoteNamesHTTP)
						// Also only keep the first 7 items.
						// Get the first 7 archive names
						var archiveNames []string
						for _, release := range remoteHTTPItems.index.Releases {
							for archiveName := range release {
								archiveNames = append(archiveNames, archiveName)
							}
						}
						sort.Strings(archiveNames)
						archiveNamesSet := sets.Set[string]{}.Insert(archiveNames[:7]...)
						// Delete all other archives
						for _, release := range remoteHTTPItems.index.Releases {
							for archiveName := range release {
								if !archiveNamesSet.Has(archiveName) {
									delete(release, archiveName)
								}
							}
						}
					})
					It("should sort local & remote by version", func() {
						env.Version = versions.AnyVersion
						env.Platform.Platform = versions.Platform{OS: "*", Arch: "*"}
						wf.List{}.Do(env)

						Expect(listFields()).To(Equal([][]string{
							{"(installed)", "v1.17.9", "linux/amd64"},
							{"(installed)", "v1.16.2", "ifonlysingularitywasstillathing/amd64"},
							{"(installed)", "v1.16.2", "linux/yourimagination"},
							{"(installed)", "v1.16.1", "linux/amd64"},
							{"(installed)", "v1.16.0", "linux/amd64"},
							{"(installed)", "v1.14.26", "hyperwarp/pixiedust"},
							{"(installed)", "v1.14.26", "linux/amd64"},
							{"(available)", "v1.11.1", "potato/cherrypie"},
							{"(available)", "v1.11.0", "darwin/amd64"},
							{"(available)", "v1.11.0", "linux/amd64"},
							{"(available)", "v1.10.1", "darwin/amd64"},
							{"(available)", "v1.10.1", "linux/amd64"},
						}))
					})
				})
				It("should skip non-matching remote contents", func() {
					env.Version.Selector = versions.PatchSelector{
						Major: 1, Minor: 16, Patch: versions.AnyPoint,
					}
					env.Platform.Arch = "*"
					wf.List{}.Do(env)

					Expect(listFields()).To(Equal([][]string{
						{"(installed)", "v1.16.2", "linux/yourimagination"},
						{"(installed)", "v1.16.1", "linux/amd64"},
						{"(installed)", "v1.16.0", "linux/amd64"},
						{"(available)", "v1.16.4", "linux/amd64"},
					}))
				})
			})
		})

		Describe("cleanup", func() {
			BeforeEach(func() {
				server.Close() // ensure no network
				flow := wf.Cleanup{}
				env.Version = versions.AnyVersion
				env.Platform.Arch = "*"
				flow.Do(env)
			})

			It("should remove matching versions from the store & keep non-matching ones", func() {
				entries, err := env.FS.ReadDir(".teststore/k8s")
				Expect(err).NotTo(HaveOccurred(), "should be able to read the store")
				Expect(entries).To(ConsistOf(
					WithTransform(fs.FileInfo.Name, Equal("1.16.2-ifonlysingularitywasstillathing-amd64")),
					WithTransform(fs.FileInfo.Name, Equal("1.14.26-hyperwarp-pixiedust")),
				))
			})
		})

		Describe("sideload", func() {
			var (
				flow wf.Sideload
			)

			var expectedPrefix string
			if testMode == gcsMode {
				// remote version fake contents are prefixed by the
				// name for easier debugging, so we can use that here
				expectedPrefix = remoteVersionsGCS[0].meta.Name
			}
			if testMode == httpMode {
				// hard coding to one of the archives in remoteVersionsHTTP as we can't pick the "first" of a map.
				expectedPrefix = "envtest-v1.10-darwin-amd64.tar.gz"
			}

			BeforeEach(func() {
				server.Close() // ensure no network
				var content []byte
				if testMode == gcsMode {
					content = remoteVersionsGCS[0].contents
				}
				if testMode == httpMode {
					content = remoteVersionsHTTP.contents[expectedPrefix]
				}
				flow.Input = bytes.NewReader(content)
				flow.PrintFormat = envp.PrintPath
			})
			It("should initialize the store if it doesn't exist", func() {
				env.Version.Selector = ver(1, 10, 0)
				Expect(env.FS.RemoveAll(testStorePath)).To(Succeed())
				flow.Do(env)
				Expect(env.FS.Stat(testStorePath)).NotTo(BeNil())
			})
			It("should fail if a non-concrete version is given", func() {
				defer shouldHaveError()
				env.Version = versions.LatestVersion
				flow.Do(env)
			})
			It("should fail if a non-concrete platform is given", func() {
				defer shouldHaveError()
				env.Version.Selector = ver(1, 10, 0)
				env.Platform.Arch = "*"
				flow.Do(env)
			})
			It("should load the given gizipped tarball into our store as the given version", func() {
				env.Version.Selector = ver(1, 10, 0)
				flow.Do(env)
				baseName := env.Platform.BaseName(*env.Version.AsConcrete())
				expectedPath := filepath.Join(".teststore/k8s", baseName, "some-file")
				outContents, err := env.FS.ReadFile(expectedPath)
				Expect(err).NotTo(HaveOccurred(), "should be able to load the unzipped file")
				Expect(string(outContents)).To(HavePrefix(expectedPrefix), "should have the debugging prefix")
			})
		})
	})
}
