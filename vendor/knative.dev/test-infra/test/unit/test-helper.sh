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


# Useful exit code
readonly SUCCESS=0
readonly FAILURE=1

# Call a function and verify its return value and output.
# Parameters: $1 - expected return code.
#             $2 - expected output ("" if no output is expected)
#             $3..$n - function to call and its parameters.
function test_function() {
  local expected_retcode=$1
  local expected_string=$2
  local output="$(mktemp)"
  local output_code="$(mktemp)"
  shift 2
  echo -n "$(trap '{ echo $? > ${output_code}; }' EXIT ; "$@")" &> ${output}
  local retcode=$(cat ${output_code})
  if [[ ${retcode} -ne ${expected_retcode} ]]; then
    cat ${output}
    echo "Return code ${retcode} doesn't match expected return code ${expected_retcode}"
    return 1
  fi
  if [[ -n "${expected_string}" ]]; then
    local found=1
    grep "${expected_string}" ${output} > /dev/null || found=0
    if (( ! found )); then
      cat ${output}
      echo "String '${expected_string}' not found"
      return 1
    fi
  else
    if [[ -s ${output} ]]; then
      ls ${output}
      cat ${output}
      echo "Unexpected output"
      return 1
    fi
  fi
  echo "'$@' returns code ${expected_retcode} and output matches with expected"
}

# Test helper that calls two functions in sequence.
# Parameters: $1 - function to call first.
#             $2 - function to call second.
#             $3..$n - parameters passed to the second function.
function call_function_pre() {
  set -e
  local init="$1"
  shift
  eval ${init}
  "$@" 2>&1
}

# Test helper that calls two functions in sequence.
# Parameters: $1 - function to call second.
#             $2 - function to call first.
#             $3..$n - parameters passed to the first function.
function call_function_post() {
  set -e
  local post="$1"
  shift
  "$@" 2>&1
  eval ${post}
}

# Run the function with gcloud mocked (does nothing and outputs nothing).
# Parameters: $1..$n - parameters passed to the function.
function mock_gcloud_function() {
  set -e
  function gcloud() {
    echo ""
  }
  "$@" 2>&1
}

# Mocks the kubectl functionality (does nothing and outputs nothing).
# Parameters: $1..$n - parameters passed to the function.
function mock_kubectl_function() {
  set -e
  function kubectl() {
    echo ""
  }
  "$@" 2>&1
}

# Convenience method to display a test failure and exit the script.
# Parameters: $1 - message to display.
function test_failed() {
  echo "$1"
  exit 1
}
