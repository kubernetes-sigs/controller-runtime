#!/usr/bin/env bash
shopt -s extglob

cr_major_pattern=":warning:|$(printf "\xe2\x9a\xa0")"
cr_minor_pattern=":sparkles:|$(printf "\xe2\x9c\xa8")"
cr_patch_pattern=":bug:|$(printf "\xf0\x9f\x90\x9b")"
cr_docs_pattern=":book:|$(printf "\xf0\x9f\x93\x96")"
cr_no_release_note_pattern=":ghost:|$(printf "\xf0\x9f\x91\xbb")"
cr_other_pattern=":running:|$(printf "\xf0\x9f\x8f\x83")"
cr_all_pattern="${cr_major_pattern}|${cr_minor_pattern}|${cr_patch_pattern}|${cr_docs_pattern}|${cr_other_pattern}"

# cr::symbol-type-raw turns :xyz: and the corresponding emoji
# into one of "major", "minor", "patch", "docs", "other", or
# "unknown", ignoring the '!'
cr::symbol-type-raw() {
    case $1 in
        @(${cr_major_pattern})?('!'))
            echo "major"
            ;;
        @(${cr_minor_pattern})?('!'))
            echo "minor"
            ;;
        @(${cr_patch_pattern})?('!'))
            echo "patch"
            ;;
        @(${cr_docs_pattern})?('!'))
            echo "docs"
            ;;
        @(${cr_no_release_note_pattern})?('!'))
            echo "no_release_note"
            ;;
        @(${cr_other_pattern})?('!'))
            echo "other"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

# cr::symbol-type turns :xyz: and the corresponding emoji
# into one of "major", "minor", "patch", "docs", "other", or
# "unknown".
cr::symbol-type() {
    local type_raw=$(cr::symbol-type-raw $1)
    if [[ ${type_raw} == "unknown" ]]; then
        echo "unknown"
        return
    fi

    if [[ $1 == *'!' ]]; then
        echo "major"
        return
    fi

    echo ${type_raw}
}

# git::is-release-branch-name checks if its argument is a release branch name
# (release-0.Y or release-X).
git::is-release-branch-name() {
    [[ ${1-must specify release branch name to check} =~ release-((0\.[[:digit:]])|[[:digit:]]+) ]]
}

# git::ensure-release-branch checks that we're on a release branch
git::ensure-release-branch() {
    local current_branch=$(git rev-parse --abbrev-ref HEAD)
    if ! git::is-release-branch-name ${current_branch}; then
        echo "branch ${current_branch} does not appear to be a release branch (release-X)" >&2
        exit 1
    fi
}

# git::export-version-from-branch outputs the current version
# for the given branch (as the argument) as exported variables
# (${maj,min,patch}_ver, last_tag).
git::export-version-from-branch() {
    local target_branch=${1?must specify a branch}
    local current_branch=$(git branch --show-current -q)

    local expected_maj_ver
    local expected_min_ver
    if [[ ${target_branch} =~ release-0.([[:digit:]]+) ]]; then
        expected_maj_ver=0
        expected_min_ver=${BASH_REMATCH[1]}
    elif [[ ${target_branch} =~ release-([[:digit:]]+) ]]; then
        expected_maj_ver=${BASH_REMATCH[1]}
    else
        echo "branch ${target_branch} does not appear to be for a release -- it should be release-X or release-0.Y" >&2
        exit 1
    fi

    local tag_pattern='v([[:digit:]]+).([[:digit:]]+).([[:digit:]]+)'

    git checkout -q ${target_branch}

    # make sure we've got a tag that matches *some* release
    last_tag=$(git describe --tags --abbrev=0) # try to fetch just the "current" tag name
    if [[ ! ${last_tag} =~ ${tag_pattern} ]]; then
        # it's probably for a previous version
        echo "tag ${last_tag} does not appear to be for a release -- it should be vX.Y.Z" >&2
        git checkout -q ${current_branch}
        exit 1
    fi

    export min_ver=${BASH_REMATCH[2]}
    export patch_ver=${BASH_REMATCH[3]}
    export maj_ver=${BASH_REMATCH[1]}
    export last_tag=${last_tag}

    if ${2:-1} && ([[ ${maj_ver} != ${expected_maj_ver} ]] || [[ ${maj_ver} == 0 && ${min_ver} != ${expected_min_ver} ]]); then
        echo "tag ${last_tag} does not appear to be for a the right release (${target_branch})" >&2
        git checkout ${current_branch}
        exit 1
    fi

    git checkout -q ${current_branch}
}

# git::export-current-version outputs the current version
# as exported variables (${maj,min,patch}_ver, last_tag) after
# checking that we're on the right release branch.
git::export-current-version() {
    # make sure we're on a release branch
    git::ensure-release-branch

    # deal with the release-0.1 branch, or similar
    local release_ver=${BASH_REMATCH[1]}
    local expected_maj_ver=${release_ver}
    if [[ ${expected_maj_ver} =~ 0\.([[:digit:]]+) ]]; then
        expected_maj_ver=0
        local expected_min_ver=${BASH_REMATCH[1]}
    fi

    git::export-version-from-branch "release-${release_ver}" false

    local last_tag_branch=""
    if [[ ${maj_ver} == "0" && ${min_ver} -eq $((expected_min_ver-1)) ]]; then
        echo "most recent tag is a release behind (${last_tag}), checking previous release branch to be safe" >&2
        last_tag_branch="release-0.${min_ver}"
    elif [[ ${maj_ver} -eq $((expected_maj_ver-1)) ]]; then
        echo "most recent tag is a release behind (${last_tag}), checking previous release branch to be safe" >&2
        last_tag_branch="release-${maj_ver}"
    fi

    if [[ -n "${last_tag_branch}" ]]; then
        git::export-version-from-branch ${last_tag_branch} true
    fi
}

# git::next-version figures out the next version to tag
# (it also sets the current version variables to the current version)
git::next-version() {
    git::export-current-version

    local feature_commits=$(git rev-list ${last_tag}..${end_range} --grep="${cr_minor_pattern}")
    local breaking_commits=$(git rev-list ${last_tag}..${end_range} --grep="${cr_major_pattern}")

    if [[ -z ${breaking_commits} && ${maj_ver} > 0 ]]; then
        local next_ver="v$(( maj_ver + 1 )).0.0"
    elif [[ -z ${feature_commits} ]]; then
        local next_ver="v${maj_ver}.$(( min_ver + 1 )).0"
    else
        local next_ver="v${maj_ver}.${min_ver}.$(( patch_ver + 1 ))"
    fi

    echo "${next_ver}"
}
