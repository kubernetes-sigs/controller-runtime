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

go vet ${MOD_OPT} ./...

header_text "running golangci-lint"

golangci-lint run --disable-all \
    --deadline 5m \
    --enable=misspell \
    --enable=structcheck \
    --enable=golint \
    --enable=deadcode \
    --enable=errcheck \
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
    --enable=goimports \
    ./pkg/... ./examples/... .

# TODO: Enable these as we fix them to make them pass
#    --enable=gosec \
#    --enable=maligned \
#    --enable=safesql \

header_text "running dep check"
dep check -skip-vendor  # vendor is maintained by go modules
GO111MODULES=on go list -mod=readonly ./...
