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

source $(dirname ${BASH_SOURCE})/common.sh

header_text "running go vet"

#go vet ./...

# go get is broken for golint.  re-enable this once it is fixed.
#header_text "running golint"
#
#golint -set_exit_status ./pkg/...

header_text "creating config"
echo "
linters-settings:
  linters-settings:
  dupl:
    threshold: 400
    min-len: 3
  lll:
    line-length: 170
    tab-width: 1" > /tmp/.golangci-lint-config.yml

header_text "running golangci-lint"

golangci-lint run --disable-all \
    --config /tmp/.golangci-lint-config.yml \
    --deadline 5m \
    --enable=misspell \
    --enable=structcheck \
    --enable=golint \
    --enable=deadcode \
    --enable=goimports \
    --enable=varcheck \
    --enable=goconst \
    --enable=unparam \
    --enable=ineffassign \
    --enable=nakedret \
    --enable=interfacer \
    --enable=misspell \
    --enable=gocyclo \
    --enable=lll \
    --enable=dupl \
    --skip-dirs=atomic \
    --enable=goimports \
    ./pkg/... ./examples/... .
# TODO: Enable these as we fix them to make them pass
#    --enable=errcheck \
#    --enable=gosec \
#    --enable=maligned \
#    --enable=safesql \

header_text "running dep check"
dep check
