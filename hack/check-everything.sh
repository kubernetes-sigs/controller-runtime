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
source ${hack_dir}/setup-envtest.sh

tmp_root=/tmp
kb_root_dir=$tmp_root/kubebuilder

# Skip fetching and untaring the tools by setting the SKIP_FETCH_TOOLS variable
# in your environment to any value:
#
# $ SKIP_FETCH_TOOLS=1 ./check-everything.sh
#
# If you skip fetching tools, this script will use the tools already on your
# machine, but rebuild the kubebuilder and kubebuilder-bin binaries.
SKIP_FETCH_TOOLS=${SKIP_FETCH_TOOLS:-""}


if [ -z "$SKIP_FETCH_TOOLS" ]; then
  header_text "fetching envtest tools"
  fetch_envtest_tools "$kb_root_dir"
  fetch_envtest_tools "${hack_dir}/../pkg/internal/testing/integration/assets"
fi

header_text "setting up envtest environment"
setup_envtest_env "$kb_root_dir"

${hack_dir}/verify.sh
${hack_dir}/test-all.sh

header_text "confirming examples compile (via go install)"
go install ${MOD_OPT} ./examples/builtins
go install ${MOD_OPT} ./examples/crd

echo "passed"
exit 0
