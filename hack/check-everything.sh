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

set -e

hack_dir=$(dirname ${BASH_SOURCE})
source ${hack_dir}/common.sh

k8s_version=1.14.1
goarch=amd64
goos="unknown"

if [[ "$OSTYPE" == "linux-gnu" ]]; then
  goos="linux"
elif [[ "$OSTYPE" == "darwin"* ]]; then
  goos="darwin"
fi

if [[ "$goos" == "unknown" ]]; then
  echo "OS '$OSTYPE' not supported. Aborting." >&2
  exit 1
fi

tmp_root=/tmp
kb_root_dir=$tmp_root/kubebuilder

# Skip fetching and untaring the tools by setting the SKIP_FETCH_TOOLS variable
# in your environment to any value:
#
# $ SKIP_FETCH_TOOLS=1 ./test.sh
#
# If you skip fetching tools, this script will use the tools already on your
# machine, but rebuild the kubebuilder and kubebuilder-bin binaries.
SKIP_FETCH_TOOLS=${SKIP_FETCH_TOOLS:-""}

# fetch k8s API gen tools and make it available under kb_root_dir/bin.
function fetch_kb_tools {
  header_text "fetching tools"
  kb_tools_archive_name="kubebuilder-tools-$k8s_version-$goos-$goarch.tar.gz"
  kb_tools_download_url="https://storage.googleapis.com/kubebuilder-tools/$kb_tools_archive_name"

  kb_tools_archive_path="$tmp_root/$kb_tools_archive_name"
  if [ ! -f $kb_tools_archive_path ]; then
    curl -sL ${kb_tools_download_url} -o "$kb_tools_archive_path"
  fi
  tar -zvxf "$kb_tools_archive_path" -C "$tmp_root/"
}

function is_installed {
  if [ command -v $1 &>/dev/null ]; then
    return 0
  fi
  return 1
}

function fetch_go_tools {
  header_text "Checking for gometalinter.v2"
  if ! is_installed golangci-lint; then
    header_text "Installing golangci-lint"
    GO111MODULE=on go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.15.0
  fi

  header_text "Checking for dep"
  if ! is_installed dep; then
    header_text "Installing dep"
    # can't install dep with modules from source due to an issue with semver,
    # so install from the compiled binary instead
    curl -sL -o $(go env GOPATH)/bin/dep https://github.com/golang/dep/releases/download/0.5.2/dep-$(go env GOOS)-$(go env GOARCH)
  fi
}

header_text "using tools"

if [ -z "$SKIP_FETCH_TOOLS" ]; then
  fetch_go_tools
  fetch_kb_tools
fi

setup_envs

${hack_dir}/verify.sh
${hack_dir}/test-all.sh

header_text "confirming examples compile (via go install)"
go install ${MOD_OPT} ./examples/builtins
go install ${MOD_OPT} ./examples/crd

echo "passed"
exit 0
