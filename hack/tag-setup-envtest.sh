#!/usr/bin/env bash

#  Copyright 2026 The Kubernetes Authors.
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

set -euo pipefail

readonly PREFIX="${1:-tools/setup-envtest/}"
readonly REF="${2:-${GITHUB_REF}}"
readonly SHA="${3:-${GITHUB_SHA}}"
readonly REPO="${4:-${GITHUB_REPOSITORY}}"

ORIGINAL_TAG="${REF#refs/tags/}"
NEW_TAG="${PREFIX}${ORIGINAL_TAG}"
if git show-ref --verify --quiet "refs/tags/${NEW_TAG}"; then
  echo "Tag ${NEW_TAG} already exists, nothing to do."
  exit 0
fi

echo "Creating new tag: '${NEW_TAG}' at SHA: ${SHA}"

gh api \
  --method POST \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "/repos/${REPO}/git/refs" \
  -f ref="refs/tags/${NEW_TAG}" \
  -f sha="${SHA}"
echo
echo "Successfully created ${NEW_TAG}"
