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

set -euo pipefail

source $(dirname $0)/../vendor/knative.dev/hack/library.sh

KOURIER_GATEWAY_NAMESPACE=kourier-system
KOURIER_CONTROL_NAMESPACE=knative-serving
TEST_NAMESPACE=serving-tests
LATEST_RELEASE_VERSION=$(latest_version)

$(dirname $0)/upload-test-images.sh

echo ">> Setup test resources"
ko apply -f test/config

# Exclude the control-plane node, which doesn't seem to expose the nodeport service.
IPS=( $(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}') )

export "GATEWAY_OVERRIDE=kourier"
export "GATEWAY_NAMESPACE_OVERRIDE=${KOURIER_GATEWAY_NAMESPACE}"

echo "Install the latest release Kourier version ${LATEST_RELEASE_VERSION}"
kubectl apply -f "https://github.com/knative-sandbox/net-kourier/releases/download/${LATEST_RELEASE_VERSION}/release.yaml" --dry-run=client -o yaml | \
  sed 's/LoadBalancer/NodePort/g' | \
  kubectl apply -f -

echo "Wait for all deployments to be up"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/net-kourier-controller
kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/3scale-kourier-gateway

# Remove the following files in case we failed to clean them up in an earlier test.
rm -f /tmp/prober-signal

echo "Running prober test"
go test -count=1 -timeout=20m -tags=probe ./test/upgrade/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev &
PROBER_PID=$!
echo "Prober PID is ${PROBER_PID}"

# Wait for the ingress to become ready
until [[ $(kubectl -n "${TEST_NAMESPACE}" get ingresses.networking.internal.knative.dev -oname | wc -l) == 1 ]]; do sleep 1; done
kubectl -n "${TEST_NAMESPACE}" wait --timeout=300s --for=condition=Ready ingresses.networking.internal.knative.dev --all

echo "Install the current Kourier version"
ko resolve -f config | \
  sed 's/LoadBalancer/NodePort/g' | \
  kubectl apply -f -

echo "Wait for the deployments to roll over"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller
kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" rollout status deployment/3scale-kourier-gateway

echo "Wait for some more traffic to flow"
sleep 10

# The probe tests are blocking on the following files to know when it should exit.
echo "done" > /tmp/prober-signal

wait ${PROBER_PID}
