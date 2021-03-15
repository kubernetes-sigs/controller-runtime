package process_test

import (
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/controller-runtime/pkg/internal/testing/process"
)

var _ = Describe("Arguments Templates", func() {
	It("templates URLs", func() {
		templates := []string{
			"plain URL: {{ .SomeURL }}",
			"method on URL: {{ .SomeURL.Hostname }}",
			"empty URL: {{ .EmptyURL }}",
			"handled empty URL: {{- if .EmptyURL }}{{ .EmptyURL }}{{ end }}",
		}
		data := struct {
			SomeURL  *url.URL
			EmptyURL *url.URL
		}{
			&url.URL{Scheme: "https", Host: "the.host.name:3456"},
			nil,
		}

		out, err := RenderTemplates(templates, data)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeEquivalentTo([]string{
			"plain URL: https://the.host.name:3456",
			"method on URL: the.host.name",
			"empty URL: &lt;nil&gt;",
			"handled empty URL:",
		}))
	})

	It("templates strings", func() {
		templates := []string{
			"a string: {{ .SomeString }}",
			"empty string: {{- .EmptyString }}",
		}
		data := struct {
			SomeString  string
			EmptyString string
		}{
			"this is some random string",
			"",
		}

		out, err := RenderTemplates(templates, data)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeEquivalentTo([]string{
			"a string: this is some random string",
			"empty string:",
		}))
	})

	It("has no access to unexported fields", func() {
		templates := []string{
			"this is just a string",
			"this blows up {{ .test }}",
		}
		data := struct{ test string }{"ooops private"}

		out, err := RenderTemplates(templates, data)
		Expect(out).To(BeEmpty())
		Expect(err).To(MatchError(
			ContainSubstring("is an unexported field of struct"),
		))
	})

	It("errors when field cannot be found", func() {
		templates := []string{"this does {{ .NotExist }}"}
		data := struct{ Unused string }{"unused"}

		out, err := RenderTemplates(templates, data)
		Expect(out).To(BeEmpty())
		Expect(err).To(MatchError(
			ContainSubstring("can't evaluate field"),
		))
	})
})

type plainDefaults map[string][]string

func (d plainDefaults) DefaultArgs() map[string][]string {
	return d
}

var _ = Describe("Arguments", func() {
	Context("when appending", func() {
		It("should copy from defaults when appending for the first time", func() {
			args := EmptyArguments().
				Append("some-key", "val3")
			Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"val1", "val2", "val3"}))
		})

		It("should not copy from defaults if the flag has been disabled previously", func() {
			args := EmptyArguments().
				Disable("some-key").
				Append("some-key", "val3")
			Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"val3"}))
		})

		It("should only copy defaults the first time", func() {
			args := EmptyArguments().
				Append("some-key", "val3", "val4").
				Append("some-key", "val5")
			Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"val1", "val2", "val3", "val4", "val5"}))
		})

		It("should not copy from defaults if the flag has been previously overridden", func() {
			args := EmptyArguments().
				Set("some-key", "vala").
				Append("some-key", "valb", "valc")
			Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"vala", "valb", "valc"}))
		})

		Context("when explicitly overriding defaults", func() {
			It("should not copy from defaults, but should append to previous calls", func() {
				args := EmptyArguments().
					AppendNoDefaults("some-key", "vala").
					AppendNoDefaults("some-key", "valb", "valc")
				Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"vala", "valb", "valc"}))
			})

			It("should not copy from defaults, but should respect previous appends' copies", func() {
				args := EmptyArguments().
					Append("some-key", "vala").
					AppendNoDefaults("some-key", "valb", "valc")
				Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"val1", "val2", "vala", "valb", "valc"}))
			})

			It("should not copy from defaults if the flag has been previously appended to ignoring defaults", func() {
				args := EmptyArguments().
					AppendNoDefaults("some-key", "vala").
					Append("some-key", "valb", "valc")
				Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"vala", "valb", "valc"}))
			})
		})
	})

	It("should ignore defaults when overriding", func() {
		args := EmptyArguments().
			Set("some-key", "vala")
		Expect(args.Get("some-key").Get([]string{"val1", "val2"})).To(Equal([]string{"vala"}))
	})

	It("should allow directly setting the argument value for custom argument types", func() {
		args := EmptyArguments().
			SetRaw("custom-key", commaArg{"val3"}).
			Append("custom-key", "val4")
		Expect(args.Get("custom-key").Get([]string{"val1", "val2"})).To(Equal([]string{"val1,val2,val3,val4"}))
	})

	Context("when rendering flags", func() {
		It("should not render defaults for disabled flags", func() {
			defs := map[string][]string{
				"some-key":  []string{"val1", "val2"},
				"other-key": []string{"val"},
			}
			args := EmptyArguments().
				Disable("some-key")
			Expect(args.AsStrings(defs)).To(ConsistOf("--other-key=val"))
		})

		It("should render name-only flags as --key", func() {
			args := EmptyArguments().
				Enable("some-key")
			Expect(args.AsStrings(nil)).To(ConsistOf("--some-key"))
		})

		It("should render multiple values as --key=val1, --key=val2", func() {
			args := EmptyArguments().
				Append("some-key", "val1", "val2").
				Append("other-key", "vala", "valb")
			Expect(args.AsStrings(nil)).To(ConsistOf("--other-key=valb", "--other-key=vala", "--some-key=val1", "--some-key=val2"))
		})

		It("should read from defaults if the user hasn't set a value for a flag", func() {
			defs := map[string][]string{
				"some-key": []string{"val1", "val2"},
			}
			args := EmptyArguments().
				Append("other-key", "vala", "valb")
			Expect(args.AsStrings(defs)).To(ConsistOf("--other-key=valb", "--other-key=vala", "--some-key=val1", "--some-key=val2"))
		})

		It("should not render defaults if the user has set a value for a flag", func() {
			defs := map[string][]string{
				"some-key": []string{"val1", "val2"},
			}
			args := EmptyArguments().
				Set("some-key", "vala")
			Expect(args.AsStrings(defs)).To(ConsistOf("--some-key=vala"))
		})
	})
})

type commaArg []string

func (a commaArg) Get(defs []string) []string {
	// not quite, but close enough
	return []string{strings.Join(defs, ",") + "," + strings.Join(a, ",")}
}
func (a commaArg) Append(vals ...string) Arg {
	return commaArg(append(a, vals...))
}
