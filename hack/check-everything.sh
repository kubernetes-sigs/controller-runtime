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
source ${hack_dir}/setup-envtest.sh

tmp_root=/tmp
kb_root_dir=$tmp_root/kubebuilder

ENVTEST_K8S_VERSION=${ENVTEST_K8S_VERSION:-"1.19.2"}

fetch_envtest_tools "$kb_root_dir"
fetch_envtest_tools "${hack_dir}/../pkg/internal/testing/integration/assets"
setup_envtest_env "$kb_root_dir"

${hack_dir}/verify.sh
${hack_dir}/test-all.sh

header_text "confirming examples compile (via go install)"
go install ${MOD_OPT} ./examples/builtins
go install ${MOD_OPT} ./examples/crd
go install ${MOD_OPT} ./examples/configfile/builtin
go install ${MOD_OPT} ./examples/configfile/custom

echo "passed"
exit 0
