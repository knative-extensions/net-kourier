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

# Fake we're in a Prow job, if running locally.
[[ -z "${PROW_JOB_ID:-}" ]] && PROW_JOB_ID=123
[[ -z "${PULL_PULL_SHA:-}" ]] && PULL_PULL_SHA=456
[[ -z "${ARTIFATCS:-}" ]] && ARTIFACTS=/tmp

source $(dirname $0)/test-helper.sh

set -e

function init_test_env() {
  source $(dirname $0)/../../scripts/presubmit-tests.sh

  # Mock external tools for testing purposes.
  function list_changed_files() {
    echo "foobar.go"
  }

  function markdown-link-check() {
    return 0
  }

  function mdl() {
    return 0
  }

  function check_links_in_markdown() {
    return 0
  }

  function default_build_test_runner() {
    echo "EXECUTING default_build_test_runner"
    return 0
  }
}

# Helper functions.

function mock_presubmit_runners() {
  RETURN_CODE="${1:-0}"

  RAN_BUILD_TESTS=0
  RAN_UNIT_TESTS=0
  RAN_INTEGRATION_TESTS=0
  PRE_BUILD_TESTS=0
  POST_BUILD_TESTS=0
  PRE_UNIT_TESTS=0
  POST_UNIT_TESTS=0
  PRE_INTEGRATION_TESTS=0
  POST_INTEGRATION_TESTS=0
  function pre_build_tests() {
    PRE_BUILD_TESTS=1
    return ${RETURN_CODE}
  }
  function build_tests() {
    RAN_BUILD_TESTS=1
    return ${RETURN_CODE}
  }
  function post_build_tests() {
    POST_BUILD_TESTS=1
    return ${RETURN_CODE}
  }
  function pre_unit_tests() {
    PRE_UNIT_TESTS=1
    return ${RETURN_CODE}
  }
  function unit_tests() {
    RAN_UNIT_TESTS=1
    return ${RETURN_CODE}
  }
  function post_unit_tests() {
    POST_UNIT_TESTS=1
    return ${RETURN_CODE}
  }
  function pre_integration_tests() {
    PRE_INTEGRATION_TESTS=1
    return ${RETURN_CODE}
  }
  function integration_tests() {
    RAN_INTEGRATION_TESTS=1
    return ${RETURN_CODE}
  }
  function post_integration_tests() {
    POST_INTEGRATION_TESTS=1
    return ${RETURN_CODE}
  }
}

function test_custom_runners_all() {
  mock_presubmit_runners
  function check_results() {
    (( PRE_BUILD_TESTS )) || test_failed "Pre build tests did not run"
    (( RAN_BUILD_TESTS )) || test_failed "Build tests did not run"
    (( POST_BUILD_TESTS )) || test_failed "Post build tests did not run"
    (( PRE_UNIT_TESTS )) || test_failed "Pre unit tests did not run"
    (( RAN_UNIT_TESTS )) || test_failed "Unit tests did not run"
    (( POST_UNIT_TESTS )) || test_failed "Post unit tests did not run"
    (( PRE_INTEGRATION_TESTS )) || test_failed "Pre integration tests did not run"
    (( RAN_INTEGRATION_TESTS )) || test_failed "Custom integration tests did not run"
    (( POST_INTEGRATION_TESTS )) || test_failed "Post integration tests did not run"
    echo "Test passed"
  }
}

function test_custom_runners_basic() {
  mock_presubmit_runners
  unset pre_build_tests
  unset post_build_tests
  unset pre_unit_tests
  unset post_unit_tests
  unset pre_integration_tests
  unset post_integration_tests
  function check_results() {
    (( ! PRE_BUILD_TESTS )) || test_failed "Pre build tests did run"
    (( RAN_BUILD_TESTS )) || test_failed "Build tests did not run"
    (( ! POST_BUILD_TESTS )) || test_failed "Post build tests did run"
    (( ! PRE_UNIT_TESTS )) || test_failed "Pre unit tests did run"
    (( RAN_UNIT_TESTS )) || test_failed "Unit tests did not run"
    (( ! POST_UNIT_TESTS )) || test_failed "Post unit tests did run"
    (( ! PRE_INTEGRATION_TESTS )) || test_failed "Pre integration tests did run"
    (( RAN_INTEGRATION_TESTS )) || test_failed "Custom integration tests did not run"
    (( ! POST_INTEGRATION_TESTS )) || test_failed "Post integration tests did run"
    echo "Test passed"
  }
}

function test_custom_runners_fail_slow() {
  mock_presubmit_runners ${FAILURE}
  unset pre_build_tests
  unset post_build_tests
  unset pre_unit_tests
  unset post_unit_tests
  unset pre_integration_tests
  unset post_integration_tests
  function check_results() {
    (( ! PRE_BUILD_TESTS )) || test_failed "Pre build tests did run"
    (( RAN_BUILD_TESTS )) || test_failed "Build tests did not run"
    (( ! POST_BUILD_TESTS )) || test_failed "Post build tests did run"
    (( ! PRE_UNIT_TESTS )) || test_failed "Pre unit tests did run"
    (( RAN_UNIT_TESTS )) || test_failed "Unit tests did not run"
    (( ! POST_UNIT_TESTS )) || test_failed "Post unit tests did run"
    (( ! PRE_INTEGRATION_TESTS )) || test_failed "Pre integration tests did run"
    (( RAN_INTEGRATION_TESTS )) || test_failed "Custom integration tests did not run"
    (( ! POST_INTEGRATION_TESTS )) || test_failed "Post integration tests did run"
    echo "Test failed slow"
  }
}

function test_custom_runners_fail_fast() {
  PRESUBMIT_TEST_FAIL_FAST=1
  mock_presubmit_runners ${FAILURE}
  unset pre_build_tests
  unset post_build_tests
  unset pre_unit_tests
  unset post_unit_tests
  unset pre_integration_tests
  unset post_integration_tests
  function check_results() {
    (( ! PRE_BUILD_TESTS )) || test_failed "Pre build tests did run"
    (( RAN_BUILD_TESTS )) || test_failed "Build tests did not run"
    (( ! POST_BUILD_TESTS )) || test_failed "Post build tests did run"
    (( ! PRE_UNIT_TESTS )) || test_failed "Pre unit tests did run"
    (( ! RAN_UNIT_TESTS )) || test_failed "Unit tests did run"
    (( ! POST_UNIT_TESTS )) || test_failed "Post unit tests did run"
    (( ! PRE_INTEGRATION_TESTS )) || test_failed "Pre integration tests did run"
    (( ! RAN_INTEGRATION_TESTS )) || test_failed "Custom integration tests did run"
    (( ! POST_INTEGRATION_TESTS )) || test_failed "Post integration tests did run"
    echo "Test failed fast"
  }
}

function test_exempt_md() {
  source $(dirname $0)/../../scripts/presubmit-tests.sh

  function list_changed_files() {
    echo "README.md"
    echo "OWNERS"
    echo "foo.png"
  }
  initialize_environment
  (( IS_DOCUMENTATION_PR )) || test_failed "PR has no code"
  (( ! IS_PRESUBMIT_EXEMPT_PR )) || test_failed "README.md is not exempt"
}

function test_exempt_no_md() {
  source $(dirname $0)/../../scripts/presubmit-tests.sh

  function list_changed_files() {
    echo "OWNERS"
    echo "AUTHORS"
  }
  initialize_environment
  (( ! IS_DOCUMENTATION_PR )) || test_failed "OWNERS is not considered documentation"
  (( IS_PRESUBMIT_EXEMPT_PR )) || test_failed "OWNERS is exempt"
}

function test_exempt_md_code() {
  source $(dirname $0)/../../scripts/presubmit-tests.sh

  function list_changed_files() {
    echo "OWNERS"
    echo "README.md"
    echo "foo.go"
  }
  initialize_environment
  (( ! IS_DOCUMENTATION_PR )) || test_failed "foo.go is not documentation"
  (( ! IS_PRESUBMIT_EXEMPT_PR )) || test_failed "foo.go is not exempt"
}

function test_exempt_code() {
  source $(dirname $0)/../../scripts/presubmit-tests.sh

  function list_changed_files() {
    echo "foo.go"
    echo "foo.sh"
  }
  initialize_environment
  (( ! IS_DOCUMENTATION_PR )) || test_failed "foo.go is not documentation"
  (( ! IS_PRESUBMIT_EXEMPT_PR )) || test_failed "foo.go is not exempt"
}

function run_markdown_build_tests() {
  function list_changed_files() {
    echo "README.md"
  }
  main --build-tests
}

function run_main() {
  init_test_env
  # Keep current EXIT trap, used by `test_function`
  local current_trap="$(trap -p EXIT | cut -d\' -f2)"
  trap -- "${current_trap};check_results" EXIT
  main
}

echo ">> Testing presubmit exempt flow"

test_function ${SUCCESS} "Changed files in commit" call_function_pre test_exempt_md
test_function ${SUCCESS} "Changed files in commit" call_function_pre test_exempt_no_md
test_function ${SUCCESS} "Changed files in commit" call_function_pre test_exempt_md_code
test_function ${SUCCESS} "Changed files in commit" call_function_pre test_exempt_code

echo ">> Testing custom test runners"

test_function ${SUCCESS} "Test passed" call_function_pre test_custom_runners_all run_main
test_function ${SUCCESS} "Test passed" call_function_pre test_custom_runners_basic run_main
test_function ${FAILURE} "Test failed slow" call_function_pre test_custom_runners_fail_slow run_main
test_function ${FAILURE} "Test failed fast" call_function_pre test_custom_runners_fail_fast run_main

echo ">> Testing default test runners"

init_test_env
test_function ${SUCCESS} "BUILD TESTS PASSED" main --build-tests
test_function ${SUCCESS} "EXECUTING default_build_test_runner" main --build-tests
test_function ${SUCCESS} "BUILD TESTS PASSED" call_function_pre run_markdown_build_tests

echo ">> All tests passed"
