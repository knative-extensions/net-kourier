#!/usr/bin/env bash

# Copyright 2018 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

source $(dirname $0)/test-helper.sh
source $(dirname $0)/../../scripts/release.sh

set -e

function mock_publish_to_github() {
  set -e
  PUBLISH_TO_GITHUB=1
  TAG=sometag
  function git() {
	echo $@
  }
  function hub() {
	echo $@
  }
  publish_to_github "$@" 2>&1
}

function mock_publish_to_github_fails() {
  set -e
  PUBLISH_TO_GITHUB=1
  TAG=sometag
  function git() {
	echo $@
  }
  function hub() {
	echo $@
        return 1
  }
  publish_to_github "$@" 2>&1
}

function build_release() {
  return 0
}

echo ">> Testing helper functions"

test_function ${SUCCESS} "0.2" master_version "v0.2.1"
test_function ${SUCCESS} "0.2" master_version "0.2.1"
test_function ${SUCCESS} "1" release_build_number "v0.2.1"
test_function ${SUCCESS} "1" release_build_number "0.2.1"
test_function ${SUCCESS} "deadbeef" hash_from_tag "v20010101-deadbeef"

echo ">> Testing flag parsing"

test_function ${FAILURE} "error: missing parameter" parse_flags --version
test_function ${FAILURE} "error: version format" parse_flags --version a
test_function ${FAILURE} "error: version format" parse_flags --version 0.0
test_function ${SUCCESS} "" parse_flags --version 1.0.0

test_function ${FAILURE} "error: missing parameter" parse_flags --branch
test_function ${FAILURE} "error: branch name must be" parse_flags --branch a
test_function ${FAILURE} "error: branch name must be" parse_flags --branch 0.0
test_function ${SUCCESS} "" parse_flags --branch release-0.0

test_function ${FAILURE} "error: missing parameter" parse_flags --release-notes
test_function ${FAILURE} "error: file a doesn't" parse_flags --release-notes a
test_function ${SUCCESS} "" parse_flags --release-notes $(mktemp)

test_function ${FAILURE} "error: missing parameter" parse_flags --release-gcs
test_function ${SUCCESS} "" parse_flags --release-gcs a --publish

test_function ${FAILURE} "error: missing parameter" parse_flags --release-gcr
test_function ${SUCCESS} "" parse_flags --release-gcr a --publish

test_function ${FAILURE} "error: cannot have both --dot-release and --auto-release set simultaneously" parse_flags --dot-release --auto-release
test_function ${FAILURE} "error: cannot have both --version and --auto-release set simultaneously" parse_flags --auto-release --version 1.0.0
test_function ${FAILURE} "error: cannot have both --branch and --auto-release set simultaneously" parse_flags --auto-release --branch release-0.0

test_function ${FAILURE} "error: cannot have both --release-gcs and --release-dir set simultaneously" parse_flags --release-gcs a --release-dir b

test_function ${FAILURE} "error: missing parameter" parse_flags --from-nightly
test_function ${FAILURE} "error: nightly tag" parse_flags --from-nightly aaa

token_file=$(mktemp)
echo -e "abc " > ${token_file}
test_function ${SUCCESS} ":abc:" call_function_post "echo :\$GITHUB_TOKEN:" parse_flags --github-token ${token_file}

echo ">> Testing GCR/GCS values"

test_function ${SUCCESS} "GCR flag is ignored" parse_flags --release-gcr foo
test_function ${SUCCESS} "GCS flag is ignored" parse_flags --release-gcs foo

default_release_dir="$(ls -1 $(dirname $0)/../..)"
test_function ${SUCCESS} ":ko.local:" call_function_post "echo :\$KO_DOCKER_REPO:" parse_flags
test_function ${SUCCESS} "::" call_function_post "echo :\$RELEASE_GCS_BUCKET:" parse_flags
test_function ${SUCCESS} ":${default_release_dir}:" call_function_post "echo :\$RELEASE_DIR:" parse_flags

test_function ${SUCCESS} ":gcr.io/knative-nightly:" call_function_post "echo :\$KO_DOCKER_REPO:" parse_flags --publish
test_function ${SUCCESS} ":knative-nightly/test-infra:${default_release_dir}:" call_function_post "echo :\$RELEASE_GCS_BUCKET:\$RELEASE_DIR" parse_flags --publish
test_function ${SUCCESS} ":foo::" call_function_post "echo :\$RELEASE_DIR:\$RELEASE_GCS_BUCKET:" parse_flags --publish --release-dir foo

test_function ${SUCCESS} ":foo:" call_function_post "echo :\$KO_DOCKER_REPO:" parse_flags --release-gcr foo --publish
test_function ${SUCCESS} ":foo::" call_function_post "echo :\$RELEASE_GCS_BUCKET:\$RELEASE_DIR:" parse_flags --release-gcs foo --publish

echo ">> Testing publishing to GitHub"

test_function ${SUCCESS} "" publish_to_github
test_function 129 "usage: git tag" call_function_pre PUBLISH_TO_GITHUB=1 publish_to_github
test_function ${FAILURE} "No such file" call_function_pre PUBLISH_TO_GITHUB=1 publish_to_github a.yaml b.yaml
test_function ${SUCCESS} "release create" mock_publish_to_github $(mktemp) $(mktemp)
test_function ${FAILURE} "Cannot publish release to GitHub" mock_publish_to_github_fails $(mktemp) $(mktemp)

echo ">> Testing validation tests"

test_function ${SUCCESS} "Running release validation" run_validation_tests true
test_function ${SUCCESS} "" call_function_pre SKIP_TESTS=1 run_validation_tests true
test_function ${SUCCESS} "i_passed" run_validation_tests "echo i_passed"
test_function ${FAILURE} "validation tests failed" run_validation_tests false

echo ">> All tests passed"
