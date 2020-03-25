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

source $(dirname $0)/../../scripts/e2e-tests.sh

function parse_flags() {
  if [[ "$1" == "--smoke-test-custom-flag" ]]; then
    echo ">> All tests passed"
    exit 0
  fi
  fail_test "Unexpected flag $1 passed"
}

echo ">> Testing e2e custom flags"

initialize --smoke-test-custom-flag
