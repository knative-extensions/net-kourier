#!/usr/bin/env bash

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

# This script runs e2e tests on a local kind environment.

set -exuo pipefail

source $(dirname $0)/e2e-common.sh
echo $(latest_version)

  branch_name="$(current_branch)"
  echo $branch_name

  if [[ "$branch_name" == "main" ]] || [[ "$branch_name" == "master" ]]; then
    # For everything else use the latest release
    git tag -l "*$(git tag -l "*v[0-9]*" | cut -d '-' -f2 | sort -r --version-sort | head -n1)*"
    exit 0
  fi

  tag=""

  if [[ "$branch_name" == "release-"* ]]; then
    # Infer major, minor version from the branch name
    tag="${branch_name##release-}"
  else
    # Nearest tag with the `knative-` prefix
    tag=$(git describe --abbrev=0 --match "knative-v[0-9]*")

    # Fallback to older tag scheme vX.Y.Z
    [[ -z "${tag}" ]] && tag=$(git describe --abbrev=0 --match "v[0-9]*")

    # Drop the prefix
    tag="${tag##knative-}"
  fi

  echo $tag

  major_version="$(major_version ${tag})"
  minor_version="$(minor_version ${tag})"
  echo $major_version $minor_version


  # Hardcode the jump back from 1.0
  if [ "$major_version" = "1" ] && [ "$minor_version" = "0" ]; then
    tag='v0.26*'
  else
    # Adjust the minor down by one
    tag="*v$major_version.$(( minor_version - 1 ))*"
  fi

  echo $tag

  # Get the latest patch release for the major minor
  git tag -l "${tag}*" | sort -r --version-sort | head -n1
# KOURIER_GATEWAY_NAMESPACE=kourier-system
# KOURIER_CONTROL_NAMESPACE=knative-serving
# TEST_NAMESPACE=serving-tests

# echo $(!)

# # export KO_DOCKER_REPO=kind.local
# # export KIND_CLUSTER_NAME="kourier-integration"
# # $(dirname $0)/upload-test-images.sh

# # echo ">> Setup test resources"
# # ko apply -f test/config

# # # Exclude the control-plane node, which doesn't seem to expose the nodeport service.
# # IPS=( $(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}') )

# # export "GATEWAY_OVERRIDE=kourier"
# # export "GATEWAY_NAMESPACE_OVERRIDE=${KOURIER_GATEWAY_NAMESPACE}"

# # echo "Install the old Kourier version"
# # kubectl apply -f "https://github.com/knative-sandbox/net-kourier/releases/download/v0.19.1/release.yaml"

# # echo "Wait for all deployments to be up"
# # kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/3scale-kourier-control
# # kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/3scale-kourier-gateway

# # # Remove the following files in case we failed to clean them up in an earlier test.
# # rm -f /tmp/prober-signal

# # echo "Running prober test"
# # go test -count=1 -timeout=20m -tags=probe ./test/upgrade/... \
# #   --ingressendpoint="${IPS[0]}" \
# #   --ingressClass=kourier.ingress.networking.knative.dev &
# # PROBER_PID=$!
# # echo "Prober PID is ${PROBER_PID}"

# # # Wait for the ingress to become ready
# # until [[ $(kubectl -n "${TEST_NAMESPACE}" get ingresses.networking.internal.knative.dev -oname | wc -l) == 1 ]]; do sleep 1; done
# # kubectl -n "${TEST_NAMESPACE}" wait --timeout=300s --for=condition=Ready ingresses.networking.internal.knative.dev --all

# # echo "Install the current Kourier version"
# # export KO_DOCKER_REPO=kind.local
# # ko resolve -f config | \
# #   sed 's/LoadBalancer/NodePort/g' | \
# #   kubectl apply -f -

# # echo "Wait for the deployments to roll over"
# # kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/3scale-kourier-control
# # kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" rollout status deployment/3scale-kourier-gateway

# # echo "Wait for some more traffic to flow"
# # sleep 10

# # # The probe tests are blocking on the following files to know when it should exit.
# # echo "done" > /tmp/prober-signal

# # wait ${PROBER_PID}
