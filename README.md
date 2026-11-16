[![Go Report Card](https://goreportcard.com/badge/sigs.k8s.io/controller-runtime)](https://goreportcard.com/report/sigs.k8s.io/controller-runtime)
[![godoc](https://pkg.go.dev/badge/sigs.k8s.io/controller-runtime)](https://pkg.go.dev/sigs.k8s.io/controller-runtime)

# Kubernetes controller-runtime Project

The Kubernetes controller-runtime Project is a set of go libraries for building
Controllers. It is leveraged by [Kubebuilder](https://book.kubebuilder.io/) and
[Operator SDK](https://github.com/operator-framework/operator-sdk). Both are
a great place to start for new projects. See
[Kubebuilder's Quick Start](https://book.kubebuilder.io/quick-start.html) to
see how it can be used.

Documentation:

- [Package overview](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg)
- [Basic controller using builder](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/builder#example-Builder)
- [Creating a manager](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager#example-New)
- [Creating a controller](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/controller#example-New)
- [Examples](https://github.com/kubernetes-sigs/controller-runtime/blob/main/examples)
- [Designs](https://github.com/kubernetes-sigs/controller-runtime/blob/main/designs)

# Versioning, Maintenance, and Compatibility

The full documentation can be found at [VERSIONING.md](VERSIONING.md), but TL;DR:

Users:

- We stick to a zero major version
- We publish a minor version for each Kubernetes minor release and allow breaking changes between minor versions
- We publish patch versions as needed and we don't allow breaking changes in them

Contributors:

- All code PR must be labeled with :bug: (patch fixes), :sparkles: (backwards-compatible features), or :warning: (breaking changes)
- Breaking changes will find their way into the next major release, other changes will go into an semi-immediate patch or minor release
- For a quick PR template suggesting the right information, use one of these PR templates:
  * [Breaking Changes/Features](/.github/PULL_REQUEST_TEMPLATE/breaking_change.md)
  * [Backwards-Compatible Features](/.github/PULL_REQUEST_TEMPLATE/compat_feature.md)
  * [Bug fixes](/.github/PULL_REQUEST_TEMPLATE/bug_fix.md)
  * [Documentation Changes](/.github/PULL_REQUEST_TEMPLATE/docs.md)
  * [Test/Build/Other Changes](/.github/PULL_REQUEST_TEMPLATE/other.md)

## Compatibility

Every minor version of controller-runtime has been tested with a specific minor version of client-go. A controller-runtime minor version *may* be compatible with
other client-go minor versions, but this is by chance and neither supported nor tested. In general, we create one minor version of controller-runtime
for each minor version of client-go and other k8s.io/* dependencies.

The minimum Go version of controller-runtime is the highest minimum Go version of our Go dependencies. Usually, this will
be identical to the minimum Go version of the corresponding k8s.io/* dependencies.

Compatible k8s.io/*, client-go and minimum Go versions can be looked up in our [go.mod](go.mod) file.

|          | k8s.io/*, client-go | minimum Go version |
|----------|:-------------------:|:------------------:|
| CR v0.26 |        v0.38        |        1.27        |
| CR v0.25 |        v0.37        |        1.26        |
| CR v0.24 |        v0.36        |        1.26        |
| CR v0.23 |        v0.35        |        1.25        |
| CR v0.22 |        v0.34        |        1.24        |
| CR v0.21 |        v0.33        |        1.24        |
| CR v0.20 |        v0.32        |        1.23        |
| CR v0.19 |        v0.31        |        1.22        |
| CR v0.18 |        v0.30        |        1.22        |
| CR v0.17 |        v0.29        |        1.21        |
| CR v0.16 |        v0.28        |        1.20        |
| CR v0.15 |        v0.27        |        1.20        |

## FAQ

See [FAQ.md](FAQ.md)

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- Slack channel: [#controller-runtime](https://kubernetes.slack.com/archives/C02MRBMN00Z)
- Google Group: [kubebuilder@googlegroups.com](https://groups.google.com/forum/#!forum/kubebuilder)

## Contributing

Contributions are greatly appreciated. The maintainers actively manage the issues list, and try to highlight issues suitable for newcomers.
The project follows the typical GitHub pull request model. See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
Before starting any work, please either comment on an existing issue, or file a new one.

## Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

## AI Guidance

This project follows the [Kubernetes AI guidance][Kubernetes AI guidance]:

> Using AI tools to help write your PR is acceptable, but as the author, you are responsible for understanding every change. If you used AI tools in preparing your PR, you must disclose this in the description of your PR. For example, including “This PR was written in part with the assistance of generative AI,” in the PR description is sufficient.
>
> Listing AI tooling as a co-author, co-signing commits using an AI tool, or using the assisted-by, co-developed or similar commit trailer is not allowed.
>
> All contributions must follow the [contributions policies] and use commit messages that align with [the policy].
>
> [Large AI generated PRs] and AI generated commit messages are not allowed.
>
> Do not leave the first review of AI generated changes to the reviewers. Verify the changes (code review, testing, etc.) before submitting your PR. Reviewers may ask questions about your AI-assisted code, and if you cannot explain why a change was made, the PR will be closed.
>
> When responding to review comments, you must do so without relying on AI tools. Reviewers want to engage directly with you, not with generated responses. If you do not engage directly with reviewers, the PR will be closed.

[Kubernetes AI guidance]: https://www.kubernetes.dev/docs/guide/pull-requests/#ai-guidance
[contributions policies]: https://www.kubernetes.dev/docs/guide/contributing/
[the policy]: https://www.kubernetes.dev/docs/guide/pull-requests/#commit-message-guidelines
[Large AI generated PRs]: https://www.kubernetes.dev/docs/guide/pull-requests/#large-or-automatic-edits
