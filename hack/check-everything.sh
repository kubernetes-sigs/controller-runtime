#!/usr/bin/env bash

#  Copyright 2018 The Kubernetes Authors.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

hack_dir=$(dirname ${BASH_SOURCE})
source ${hack_dir}/common.sh

tmp_root=/tmp
kb_root_dir=$tmp_root/kubebuilder

export GOTOOLCHAIN="go$(make --silent go-version)"

# Run verification scripts.
${hack_dir}/verify.sh

# Envtest.
ENVTEST_K8S_VERSION=${ENVTEST_K8S_VERSION:-"1.32.0"}

header_text "installing envtest tools@${ENVTEST_K8S_VERSION} with setup-envtest if necessary"
tmp_bin=/tmp/cr-tests-bin
(
    # don't presume to install for the user
    cd ${hack_dir}/../tools/setup-envtest
    GOBIN=${tmp_bin} go install .
)
export KUBEBUILDER_ASSETS="$(${tmp_bin}/setup-envtest use --use-env -p path "${ENVTEST_K8S_VERSION}")"

# HACK
k8s_clone_dir=$tmp_root/kubernetes
(
  k8s_repo_url=https://github.com/kubernetes/kubernetes.git

  echo "Cloning Kube repository from $k8s_repo_url..."
  git clone $k8s_repo_url $k8s_clone_dir

  cd $k8s_clone_dir

  pr_number="132704"
  echo "Fetching pull request #$pr_number..."
  git fetch origin pull/$pr_number/head:pr-$pr_number

  echo "Checking out pull request #$pr_number..."
  git checkout pr-$pr_number

  echo "Building Kube from source code..."
  make
)
k8s_bin_dir=$(
  k8s_output_dir=${k8s_clone_dir}/_output/local/go/bin
  if [ -d "${k8s_output_dir}" ]; then
    cd ${k8s_output_dir}
    pwd
  else
    echo "Directory ${k8s_output_dir} does not exist."
    exit 1
  fi
)
echo "Replacing kube-apiserver binary from ${k8s_bin_dir} to ${KUBEBUILDER_ASSETS}"
cp -f "${k8s_bin_dir}/kube-apiserver" "${KUBEBUILDER_ASSETS}/kube-apiserver"

echo "Enabling WatchListClient feature"
export KUBE_FEATURE_WatchListClient=true

# Run tests.
${hack_dir}/test-all.sh

header_text "confirming examples compile (via go install)"
go install ${MOD_OPT} ./examples/builtins
go install ${MOD_OPT} ./examples/crd

echo "passed"
exit 0
