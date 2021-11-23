# Copyright 2020 The Knative Authors
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

# This script includes common functions for testing setup and teardown.
source $(dirname $0)/../vendor/knative.dev/hack/e2e-tests.sh

export KOURIER_CONTROL_NAMESPACE=knative-serving
export KOURIER_GATEWAY_NAMESPACE=kourier-system
export GATEWAY_NAMESPACE_OVERRIDE=${KOURIER_GATEWAY_NAMESPACE}
export GATEWAY_OVERRIDE=kourier

# Setup resources.
function test_setup() {
  echo ">> Setting up logging..."
  # Install kail if needed.
  if ! which kail >/dev/null; then
    bash <(curl -sfL https://raw.githubusercontent.com/boz/kail/master/godownloader.sh) -b "$GOPATH/bin"
  fi
  # Capture all logs.
  kail >${ARTIFACTS}/k8s.log.txt &
  local kail_pid=$!
  # Clean up kail so it doesn't interfere with job shutting down
  add_trap "kill $kail_pid || true" EXIT

  # Setting up test resources.
  echo ">> Publishing test images"
  $(dirname $0)/upload-test-images.sh || fail_test "Error uploading test images"
  echo ">> Creating test resources (test/config/)"
  ko apply ${KO_FLAGS} -f test/config/ || return 1

  # Bringing up controllers.
  echo ">> Bringing up Kourier"
  ko resolve -f config | sed 's/--log-level info/--log-level debug/g' | ko apply -f - || return 1

  # Scale up components for HA tests
  kubectl -n "${KOURIER_CONTROL_NAMESPACE}" scale deployment net-kourier-controller --replicas=2
  kubectl -n "${GATEWAY_NAMESPACE_OVERRIDE}" scale deployment 3scale-kourier-gateway --replicas=2

  # Wait for pods to be running.
  echo ">> Waiting for Kourier components to be running..."
  kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller || return 1
  kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" rollout status deployment/3scale-kourier-gateway || return 1
  wait_until_service_has_external_http_address "${KOURIER_GATEWAY_NAMESPACE}" kourier || return 1
}

# Add function call to trap
# Parameters: $1 - Function to call
#             $2...$n - Signals for trap
function add_trap() {
  local cmd=$1
  shift
  for trap_signal in $@; do
    local current_trap="$(trap -p $trap_signal | cut -d\' -f2)"
    local new_cmd="($cmd)"
    [[ -n "${current_trap}" ]] && new_cmd="${current_trap};${new_cmd}"
    trap -- "${new_cmd}" $trap_signal
  done
}
