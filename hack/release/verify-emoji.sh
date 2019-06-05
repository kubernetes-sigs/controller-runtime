#!/usr/bin/env bash

set -eu
set -o pipefail

pr_title=$(jq -r '.pull_request.title' < ${GITHUB_EVENT_PATH})

read pr_prefix rest <<<${pr_title}

source "$(dirname ${BASH_SOURCE})/common.sh"

pr_type=$(cr::symbol-type ${pr_prefix})

summary=""
conclusion="success"
if [[ ${pr_type} == "unknown" ]]; then
    summary="You must specify an emoji at the beginning of the PR to indicate what kind of change this is.\nValid emoji: ${cr_all_pattern}.\nYou specified '${pr_prefix}'.\nSee VERSIONING.md for more information."
    conclusion="failure"
else
    summary="PR is a ${pr_type} change (${pr_prefix})."
fi

# get the PR (the PR sent from the event has the base branch head as the head)
base_link=$(jq -r '.pull_request.url' < ${GITHUB_EVENT_PATH})
head_commit=$(curl -H "Authorization: Bearer ${GITHUB_TOKEN}" -H 'Accept: application/vnd.github.antiope-preview+json' -q ${base_link} | jq -r '.head.sha')
echo "head commit is ${head_commit}"

curl https://api.github.com/repos/${GITHUB_REPOSITORY}/check-runs -XPOST -H "Authorization: Bearer ${GITHUB_TOKEN}" -H 'Accept: application/vnd.github.antiope-preview+json' -H 'Content-Type: application/json' -q --data-raw '{"name": "Verify Emoji", "head_sha": "'${head_commit}'", "conclusion": "'${conclusion}'", "status": "completed", "completed_at": "'$(date -Iseconds)'", "output": {"title": "Verify Emoji", "summary": "'"${summary}"'"}}'

