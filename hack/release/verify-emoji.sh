#!/bin/bash

set -eu
set -o pipefail

pr_title=$(jq '.pull_request.title' < ${GITHUB_EVENT_PATH})

read pr_prefix rest <<<${pr_title}

source "$(dirname ${BASH_SOURCE})/common.sh"

pr_type=$(cr::symbol-type ${pr_prefix})

if [[ ${pr_type} == "unknown" ]]; then
    echo "You must specify an emoji at the beginning of the PR to indicate what kind of change this is."
    echo "Valid emoji: ${cr_all_pattern}."
    echo "See VERSIONING.md for more information."
    exit 1
fi

echo "PR is a ${pr_type} change (${pr_prefix})."
exit 0
