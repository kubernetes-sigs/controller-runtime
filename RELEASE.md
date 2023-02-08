# Release Process

The Kubernetes controller-runtime Project is released on an as-needed basis. The process is as follows:

**Note:** Releases are done from the `release-MAJOR.MINOR` branches. For PATCH releases is not required
to create a new branch you will just need to ensure that all big fixes are cherry-picked into the respective
`release-MAJOR.MINOR` branch. To know more about versioning check https://semver.org/. 

## How to do a release 

### Create the new branch and the release tag

1. Create a new branch `git checkout -b release-<MAJOR.MINOR>` from master
2. Push the new branch to the remote repository

### Following semver to create new releases

The releases should follow [semver](https://semver.org/). Following an summary to help you out know the tag version that should be created:

- PATCH versions should only have backwards compatible bug fixes (:bug:). (i.e.: 0.14.1, 0.14.2, 0.15.3)
- MINOR versions can contain all changes because this project is not a stable API (< `1.0.0`) and no MAJOR releases will be made so far. (_Otherwise, if this project were a stable API then breaking changes and not backwards compatible features could only be addressed in MAJOR releases which is not our case._) In this way, any new functionality (:sparkles:) and Breaking Changes (:warning:) requires MINOR versions releases to be addressed. (i.e. 0.15.0, 0.16.0)

**What about backports**

Backport a change for old release versions will result in a PATCH release. Therefore, only bug fixes can be backported (:bug:). Consumers who requires new functionalities can upgrade the version used in their project to use the upper versions always. However, we should not risk introduce breaking changes or regression bugs that could affected those who are updating their project with PATCH releases. 

**What about Kubernetes dependencies updates**

- PATCH Kubernetes releases (i.e. 0.26.1) can be addressed into PATCH release.
- MINOR Kubernetes releases (i.e. 0.27.0) requires a MINOR release. 

### Now, let's generate the changelog

1. Create the changelog from the new branch `release-<MAJOR.MINOR>` (`git checkout release-<MAJOR.MINOR>`). 
You will need to use the [kubebuilder-release-tools][kubebuilder-release-tools] to generate the notes. See [here][release-notes-generation]

> **Note**
> - You will need to have checkout locally from the remote repository the previous branch
> - Also, ensure that you fetch all tags from the remote `git fetch --all --tags`

### Draft a new release from GitHub

1. Create a new tag with the correct version from the new `release-<MAJOR.MINOR>` branch 
2. Add the changelog on it and publish. Now, the code source is released !

### Add a new Prow test the for the new branch release

1. Create a new prow test under [github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/controller-runtime](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/controller-runtime) 
for the new `release-<MAJOR.MINOR>` branch. (i.e. for the `0.11.0` release see the PR: https://github.com/kubernetes/test-infra/pull/25205)
2. Ping the infra PR in the controller-runtime slack channel for reviews.

### Announce the new release:

1. Publish on the Slack channel the new release, i.e:

````
:announce: Controller-Runtime v0.12.0 has been released!
This release includes a Kubernetes dependency bump to v1.24.
For more info, see the release page: https://github.com/kubernetes-sigs/controller-runtime/releases.
 :tada:  Thanks to all our contributors!
````

2. An announcement email is sent to `kubebuilder@googlegroups.com` with the subject `[ANNOUNCE] Controller-Runtime $VERSION is released`
