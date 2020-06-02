#!/bin/bash

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

source $(dirname $0)/e2e-common.sh

# Script entry point.
initialize "$@" --skip-istio-addon

failed=0

go_test_e2e -timeout=20m -parallel=12 \
   ./test/conformance \
	--ingressClass=kourier.ingress.networking.knative.dev || failed=1

# Scale up components for HA tests.
for deployment in 3scale-kourier-control 3scale-kourier-gateway; do
  kubectl -n "${KOURIER_NAMESPACE}" scale deployment "$deployment" --replicas=2
done
add_trap "for deployment in 3scale-kourier-control 3scale-kourier-gateway; do \
  kubectl -n ${KOURIER_NAMESPACE} scale deployment $deployment --replicas=1; done" SIGKILL SIGTERM SIGQUIT

# Wait for a new leader controller to prevent race conditions during service reconciliation.
wait_for_leader_controller || failed=1

# Give the controller time to sync with the rest of the system components.
sleep 30

go_test_e2e -timeout=15m -failfast -parallel=1 ./test/ha -spoofinterval="10ms" \
  --ingressClass=kourier.ingress.networking.knative.dev || failed=1

# Scale back HA components.
for deployment in 3scale-kourier-control 3scale-kourier-gateway; do
  kubectl -n "${KOURIER_NAMESPACE}" scale deployment "$deployment" --replicas=1
done

(( failed )) && dump_cluster_state
(( failed )) && fail_test

success
