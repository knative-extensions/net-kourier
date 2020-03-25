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

source $(dirname $0)/test-helper.sh
source $(dirname $0)/../../scripts/library.sh

set -e

function test_report() {
  local REPORT="$(mktemp)"
  function grepit() {
    if ! grep "$1" ${REPORT} > /dev/null; then
      cat ${REPORT}
      test_failed "**** '$1' not found"
    fi
    echo "Test report contains '$1'"
  }
  ARTIFACTS=/tmp
  report_go_test -tags=library -run $1 ./test > ${REPORT} || true
  grepit "$2"
  grepit "XML report written"
}

echo ">> Testing helper functions"

test_function ${SUCCESS} "Knative Test Infra" echo "${REPO_NAME_FORMATTED}"

test_function ${SUCCESS} "${REPO_ROOT_DIR}/test/unit/library-tests.sh" get_canonical_path test/unit/library-tests.sh
test_function ${SUCCESS} "Foo Bar" capitalize "foo bar"
test_function ${SUCCESS} ">>> Knative Test Infra controller logs:" mock_kubectl_function dump_app_logs "controller" "test-infra"

test_function ${SUCCESS} "" is_protected_gcr "gcr.io/knative-releases"
test_function ${SUCCESS} "" is_protected_gcr "gcr.io/knative-nightly"
test_function ${FAILURE} "" is_protected_gcr "gcr.io/knative-foobar"
test_function ${FAILURE} "" is_protected_gcr "gcr.io/foobar-releases"
test_function ${FAILURE} "" is_protected_gcr "gcr.io/foobar-nightly"
test_function ${FAILURE} "" is_protected_gcr ""

test_function ${SUCCESS} "" is_protected_cluster "gke_knative-tests_us-central1-f_prow"
test_function ${SUCCESS} "" is_protected_cluster "gke_knative-tests_us-west2-a_prow"
test_function ${SUCCESS} "" is_protected_cluster "gke_knative-tests_us-west2-a_foobar"
test_function ${FAILURE} "" is_protected_cluster "gke_knative-foobar_us-west2-a_prow"
test_function ${FAILURE} "" is_protected_cluster ""

test_function ${SUCCESS} "" is_protected_project "knative-tests"
test_function ${FAILURE} "" is_protected_project "knative-foobar"
test_function ${FAILURE} "" is_protected_project "foobar-tests"
test_function ${FAILURE} "" is_protected_project ""

echo ">> Testing report_go_test()"

test_report TestSucceeds "^--- PASS: TestSucceeds"
test_report TestFailsWithFatal "^fatal\s\+TestFailsWithFatal"
test_report TestFailsWithPanic "^panic: test timed out"
test_report TestFailsWithSigQuit "^SIGQUIT: quit"

echo ">> All tests passed"

