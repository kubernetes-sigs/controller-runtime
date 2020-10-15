#!/usr/bin/env bash

#  Copyright 2020 The Kubernetes Authors.
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

function get_dest_dir() {
  echo "$(realpath -m ${1:-"bin"})"
}

# fetch k8s API gen tools and make it available under $1.
function fetch_envtest_tools {
  local dest_dir="$(get_dest_dir $1)"
  local archive_cache_dir=/tmp/envtest
  local k8s_version="${ENVTEST_K8S_VERSION:-1.16.4}"
  local goarch="$(go env GOARCH)"
  local goos="$(go env GOOS)"

  if [[ "$goos" != "linux" && "$goos" != "darwin" ]]; then
    echo "OS '$goos' not supported. Aborting." >&2
    return 1
  fi

  # use the pre-existing version in the temporary folder if it matches our k8s version
  if [[ -x "${dest_dir}/kube-apiserver" ]]; then
    local version=$("${dest_dir}"/kube-apiserver --version)
    if [[ $version == *"${k8s_version}"* ]]; then
      echo "# Using cached envtest tools from '${dest_dir}'"
      return 0
    fi
  fi

  echo "# Fetching envtest@${k8s_version} into '${dest_dir}'"
  local archive_name="kubebuilder-tools-${k8s_version}-${goos}-${goarch}.tar.gz"
  local url="https://storage.googleapis.com/kubebuilder-tools/${archive_name}"

  local archive_path="$archive_cache_dir/$archive_name"
  mkdir -p "$archive_cache_dir"
  if [ ! -f $archive_path ]; then
    curl -sSL ${url} -o "$archive_path"
  fi

  mkdir -p "${dest_dir}"
  tar -C "${dest_dir}" --strip-components=2 -zvxf "$archive_path" >/dev/null
}

set -o errexit
set -o pipefail

fetch_envtest_tools $@

echo "# Run the following command to finish setting up your environment:"
echo
echo "export KUBEBUILDER_ASSETS=$(get_dest_dir $1)"
