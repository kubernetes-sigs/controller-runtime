package internal_test

import (
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"time"

	. "github.com/kubernetes-sig-testing/frameworks/integration/internal"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Start method", func() {
	It("can start a process", func() {
		processState := &ProcessState{}
		processState.Path = "bash"
		processState.Args = simpleBashScript
		processState.StartTimeout = 10 * time.Second
		processState.StartMessage = "loop 5"

		err := processState.Start()
		Expect(err).NotTo(HaveOccurred())

		Consistently(processState.Session.ExitCode).Should(BeNumerically("==", -1))
	})

	Context("when process takes too long to start", func() {
		It("returns a timeout error", func() {
			processState := &ProcessState{}
			processState.Path = "bash"
			processState.Args = simpleBashScript
			processState.StartTimeout = 200 * time.Millisecond
			processState.StartMessage = "loop 5000"

			err := processState.Start()
			Expect(err).To(MatchError(ContainSubstring("timeout")))

			Eventually(processState.Session.ExitCode).Should(Equal(143))
		})
	})

	Context("when the command cannot be started", func() {
		var (
			processState *ProcessState
		)
		BeforeEach(func() {
			processState = &ProcessState{}
			processState.Path = "/nonexistent"
		})

		It("propagates the error", func() {
			err := processState.Start()

			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		Context("but Stop() is called on it", func() {
			It("does not panic", func() {
				processState.Start()

				stoppingFailedProcess := func() {
					Expect(processState.Stop()).To(Succeed())
				}

				Expect(stoppingFailedProcess).NotTo(Panic())
			})
		})
	})
})

var _ = Describe("Stop method", func() {
	Context("when Stop() is called", func() {
		var (
			processState *ProcessState
		)
		BeforeEach(func() {
			var err error
			processState = &ProcessState{}
			processState.Session, err = gexec.Start(getSimpleCommand(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			processState.StopTimeout = 10 * time.Second
		})

		It("stops the process", func() {
			Expect(processState.Stop()).To(Succeed())
		})

		Context("multiple times", func() {
			It("does not error or panic on consecutive calls", func() {
				stoppingTheProcess := func() {
					Expect(processState.Stop()).To(Succeed())
				}
				Expect(stoppingTheProcess).NotTo(Panic())
				Expect(stoppingTheProcess).NotTo(Panic())
				Expect(stoppingTheProcess).NotTo(Panic())
			})
		})
	})

	Context("when the command cannot be stopped", func() {
		It("returns a timeout error", func() {
			var err error

			processState := &ProcessState{}
			processState.Session, err = gexec.Start(getSimpleCommand(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			processState.Session.Exited = make(chan struct{})
			processState.StopTimeout = 200 * time.Millisecond

			Expect(processState.Stop()).To(MatchError(ContainSubstring("timeout")))
		})
	})

	Context("when the directory needs to be cleaned up", func() {
		It("removes the directory", func() {
			var err error

			processState := &ProcessState{}
			processState.Session, err = gexec.Start(getSimpleCommand(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			processState.Dir, err = ioutil.TempDir("", "k8s_test_framework_")
			Expect(err).NotTo(HaveOccurred())
			processState.DirNeedsCleaning = true
			processState.StopTimeout = 200 * time.Millisecond

			Expect(processState.Stop()).To(Succeed())
			Expect(processState.Dir).NotTo(BeAnExistingFile())
		})
	})
})

var _ = Describe("DoDefaulting", func() {
	Context("when all inputs are provided", func() {
		It("passes them through", func() {
			defaults, err := DoDefaulting(
				"some name",
				&url.URL{Host: "some.host.to.listen.on"},
				"/some/dir",
				"/some/path/to/some/bin",
				20*time.Hour,
				65537*time.Millisecond,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(defaults.URL).To(Equal(url.URL{Host: "some.host.to.listen.on"}))
			Expect(defaults.Dir).To(Equal("/some/dir"))
			Expect(defaults.DirNeedsCleaning).To(BeFalse())
			Expect(defaults.Path).To(Equal("/some/path/to/some/bin"))
			Expect(defaults.StartTimeout).To(Equal(20 * time.Hour))
			Expect(defaults.StopTimeout).To(Equal(65537 * time.Millisecond))
		})
	})

	Context("when inputs are empty", func() {
		It("defaults them", func() {
			defaults, err := DoDefaulting(
				"some name",
				nil,
				"",
				"",
				0,
				0,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(defaults.Dir).To(BeADirectory())
			Expect(os.RemoveAll(defaults.Dir)).To(Succeed())
			Expect(defaults.DirNeedsCleaning).To(BeTrue())

			Expect(defaults.URL).NotTo(BeZero())
			Expect(defaults.URL.Scheme).To(Equal("http"))
			Expect(defaults.URL.Hostname()).NotTo(BeEmpty())
			Expect(defaults.URL.Port()).NotTo(BeEmpty())

			Expect(defaults.Path).NotTo(BeEmpty())

			Expect(defaults.StartTimeout).NotTo(BeZero())
			Expect(defaults.StopTimeout).NotTo(BeZero())
		})
	})

	Context("when neither name nor path are provided", func() {
		It("returns an error", func() {
			_, err := DoDefaulting(
				"",
				nil,
				"",
				"",
				0,
				0,
			)
			Expect(err).To(MatchError("must have at least one of name or path"))
		})
	})
})

var simpleBashScript = []string{
	"-c",
	`
		i=0
		while true
		do
			echo "loop $i" >&2
			let 'i += 1'
			sleep 0.2
		done
	`,
}

func getSimpleCommand() *exec.Cmd {
	return exec.Command("bash", simpleBashScript...)
}
