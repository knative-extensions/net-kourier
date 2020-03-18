#!/usr/bin/env bash

# Copyright 2019 The Knative Authors
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

# This script runs the end-to-end tests.

# If you already have a Knative cluster setup and kubectl pointing
# to it, call this script with the --run-tests arguments and it will use
# the cluster and run the tests.

# Calling this script without arguments will create a new cluster in
# project $PROJECT_ID, run the tests and delete the cluster.

source $(dirname $0)/../scripts/e2e-tests.sh

# Read metadata.json and get value for key
# Parameters: $1 - Key for metadata
function get_meta_value() {
  go run ${REPO_ROOT_DIR}/vendor/knative.dev/pkg/testutils/metahelper --get $1
}

function knative_setup() {
  start_latest_knative_serving
}

# Run a prow-cluster-operation tool
# Parameters: $1..$n - parameters passed to the tool
function run_prow_cluster_tool() {
  go run ${REPO_ROOT_DIR}/vendor/knative.dev/pkg/testutils/clustermanager/prow-cluster-operation $@
}

# Get test cluster from kubeconfig, fail if it's protected
function get_e2e_test_cluster() {
  local k8s_cluster=$(kubectl config current-context)
  [[ -z "${k8s_cluster}" ]] && abort "kubectl must have been set at this point"
  # Add protection before trapping removal
  is_protected_cluster ${k8s_cluster} && \
    abort "kubeconfig context set to ${k8s_cluster}, which is forbidden"
  echo $k8s_cluster
}

# Add function call to trap
# Parameters: $1 - Function to call
#             $2...$n - Signals for trap
function add_trap {
  local cmd=$1
  shift
  for trap_signal in $@; do
    local current_trap="$(trap -p $trap_signal | cut -d\' -f2)"
    local new_cmd="($cmd)"
    [[ -n "${current_trap}" ]] && new_cmd="${current_trap};${new_cmd}"
    trap -- "${new_cmd}" $trap_signal
  done
}

# Override create_test_cluster in scripts/e2e-tests.sh
# Create test cluster with cluster creation lib and write metadata in ${ARTIFACT}/metadata.json
function create_test_cluster() {
  # Fail fast during setup.
  set -o errexit
  set -o pipefail

  header "Creating test cluster"

  local creation_args=""
  (( SKIP_ISTIO_ADDON )) || creation_args+=" --addons istio"
  [[ -n "${GCP_PROJECT}" ]] && creation_args+=" --project ${GCP_PROJECT}"
  echo "Creating cluster with args ${creation_args}"
  run_prow_cluster_tool --create ${creation_args} || fail_test "failed creating test cluster"
  # Should have kubeconfig set already
  local k8s_cluster
  k8s_cluster=$(get_e2e_test_cluster)

  # Since calling `create_test_cluster` assumes cluster creation, removing
  # cluster afterwards.
  # TODO(chaodaiG) calling async method so that it doesn't wait
  add_trap "run_prow_cluster_tool --delete > /dev/null &" EXIT SIGINT
  set +o errexit
  set +o pipefail
}

# Override setup_test_cluster, this function is almost copy paste from original
# setup_test_cluster function, other than reading metadata from
# "${ARTIFACTS}/metadata.json" instead of from global vars
# Note: lines ending with "# NA" are "New Added", and lines ending with "# NC"
# are "New Changes" based on current setup_test_cluster
function setup_test_cluster() {
  # Fail fast during setup.
  set -o errexit
  set -o pipefail

  header "Test cluster setup"
  kubectl get nodes

  header "Setting up test cluster"

  # Run cluster-creator for acquiring existing test cluster, will fail if
  # kubeconfig isn't set or cluster doesn't exist
  run_prow_cluster_tool --get || fail_test "failed getting test cluster" # NA
  # The step above collects cluster metadata and writes to
  # ${ARTIFACTS}/metadata.json file, use this information
  echo "Cluster used for running tests: $(cat ${ARTIFACTS}/metadata.json)"
  local e2e_cluster_name=$(get_meta_value "E2E:Machine")  # NA
  local e2e_cluster_region=$(get_meta_value "E2E:Region")  # NA
  local e2e_cluster_zone=$(get_meta_value "E2E:Zone")  # NA
  local e2e_project_name=$(get_meta_value "E2E:Project")  # NA

  # Set the actual project the test cluster resides in
  # It will be a project assigned by Boskos if test is running on Prow,
  # otherwise will be ${GCP_PROJECT} set up by user.
  export E2E_PROJECT_ID=$e2e_project_name  # NC
  readonly E2E_PROJECT_ID  # NC

  local k8s_user=$(gcloud config get-value core/account)
  local k8s_cluster
  k8s_cluster=$(get_e2e_test_cluster)

  # If cluster admin role isn't set, this is a brand new cluster
  # Setup the admin role and also KO_DOCKER_REPO if it is a GKE cluster
  if [[ -z "$(kubectl get clusterrolebinding cluster-admin-binding 2> /dev/null)" && "${k8s_cluster}" =~ ^gke_.* ]]; then
    acquire_cluster_admin_role ${k8s_user} ${e2e_cluster_name} ${e2e_cluster_region} ${e2e_cluster_zone} # NC
    # Incorporate an element of randomness to ensure that each run properly publishes images.
    export KO_DOCKER_REPO=gcr.io/${E2E_PROJECT_ID}/${E2E_BASE_NAME}-e2e-img/${RANDOM}
  fi

  # Safety checks
  is_protected_gcr ${KO_DOCKER_REPO} && \
    abort "\$KO_DOCKER_REPO set to ${KO_DOCKER_REPO}, which is forbidden"

  # Use default namespace for all subsequent kubectl commands in this context
  kubectl config set-context ${k8s_cluster} --namespace=default

  echo "- gcloud project is ${E2E_PROJECT_ID}"
  echo "- gcloud user is ${k8s_user}"
  echo "- Cluster is ${k8s_cluster}"
  echo "- Docker repository is ${KO_DOCKER_REPO}"

  export KO_DATA_PATH="${REPO_ROOT_DIR}/.git"

  add_trap teardown_test_resources EXIT # NC

  # Handle failures ourselves, so we can dump useful info.
  set +o errexit
  set +o pipefail

  if (( ! SKIP_KNATIVE_SETUP )) && function_exists knative_setup; then
    # Wait for Istio installation to complete, if necessary, before calling knative_setup.
    (( ! SKIP_ISTIO_ADDON )) && (wait_until_batch_job_complete istio-system || return 1)
    knative_setup || fail_test "Knative setup failed"
  fi
  if function_exists test_setup; then
    test_setup || fail_test "test setup failed"
  fi
}

# Script entry point.

# Create cluster, this should have kubectl set
initialize $@
# Setup cluster
setup_test_cluster # NA

go_test_e2e ./test/e2e || fail_test

success
