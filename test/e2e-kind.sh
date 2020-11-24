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

KOURIER_GATEWAY_NAMESPACE=kourier-system
KOURIER_CONTROL_NAMESPACE=knative-serving

export KO_DOCKER_REPO=kind.local
export KIND_CLUSTER_NAME="kourier-integration"
$(dirname $0)/upload-test-images.sh

echo ">> Setup test resources"
ko apply -f test/config

ip=$(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}' | head -n1)

export "GATEWAY_OVERRIDE=kourier"
export "GATEWAY_NAMESPACE_OVERRIDE=${KOURIER_GATEWAY_NAMESPACE}"

echo ">> Running conformance tests"
go test -count=1 -short -timeout=20m -tags=e2e ./test/conformance/... ./test/e2e/... \
  --enable-alpha --enable-beta \
  --ingressendpoint="${ip}" \
  --ingressClass=kourier.ingress.networking.knative.dev

echo ">> Scale up components for HA tests"
kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" scale deployment 3scale-kourier-gateway --replicas=2
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" scale deployment 3scale-kourier-control --replicas=2

echo ">> Running HA tests"
go test -count=1 -timeout=15m -failfast -parallel=1 -tags=e2e ./test/ha -spoofinterval="10ms" \
  --ingressendpoint="${ip}" \
  --ingressClass=kourier.ingress.networking.knative.dev

echo ">> Scale down after HA tests"
kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" scale deployment 3scale-kourier-gateway --replicas=1
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" scale deployment 3scale-kourier-control --replicas=1

echo ">> Setup ExtAuthz"
ko apply -f test/config/extauthz
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/externalauthz
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment 3scale-kourier-control KOURIER_EXTAUTHZ_HOST=externalauthz.knative-serving:6000

echo ">> Running ExtAuthz tests"
go test -race -count=1 -timeout=20m -tags=e2e ./test/extauthz/... \
  --ingressendpoint="${ip}" \
  --ingressClass=kourier.ingress.networking.knative.dev
