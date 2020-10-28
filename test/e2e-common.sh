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
  sed 's/--log-level info/--log-level debug/g' deploy/kourier-knative.yaml | ko apply -f - || return 1

  scale_deployment 3scale-kourier-control "${KOURIER_CONTROL_NAMESPACE}"
  scale_deployment 3scale-kourier-gateway "${GATEWAY_NAMESPACE_OVERRIDE}"

  # Wait for pods to be running.
  echo ">> Waiting for Kourier components to be running..."
  wait_until_pods_running "${KOURIER_CONTROL_NAMESPACE}" || return 1
  wait_until_pods_running "${KOURIER_GATEWAY_NAMESPACE}" || return 1
  wait_until_service_has_external_http_address "${KOURIER_GATEWAY_NAMESPACE}" kourier || return 1

  # Wait for a new leader controller to prevent race conditions during service reconciliation.
  wait_for_leader_controller || failed=1
}

function scale_deployment() {
    # Make sure all pods run in leader-elected mode.
    kubectl -n "$2" scale deployment "$1" --replicas=0 || failed=1
    # Give it time to kill the pods.
    sleep 5
    # Scale up components for HA tests
    kubectl -n "$2" scale deployment "$1" --replicas=2 || failed=1
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

function wait_for_leader_controller() {
  echo -n "Waiting for leader Controller"
  for i in {1..150}; do # timeout after 5 minutes
    local leader=$(kubectl get lease -n "${KOURIER_CONTROL_NAMESPACE}" -ojsonpath='{.items[*].spec.holderIdentity}' | cut -d"_" -f1 | grep "^3scale-kourier-control-" | head -1)
    # Make sure the leader pod exists.
    if [ -n "${leader}" ] && kubectl get pod "${leader}" -n "${KOURIER_CONTROL_NAMESPACE}" >/dev/null 2>&1; then
      echo -e "\nNew leader Controller has been elected"
      return 0
    fi
    echo -n "."
    sleep 2
  done
  echo -e "\n\nERROR: timeout waiting for leader controller"
  return 1
}
