#!/usr/bin/env bash

set -e
set -u

export GO111MODULE=on
./bin/install-test-dependencies.sh
./bin/pre-commit.sh
