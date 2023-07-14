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
TEST_NAMESPACE=serving-tests
CLUSTER_SUFFIX=${CLUSTER_SUFFIX:-cluster.local}

$(dirname $0)/upload-test-images.sh

echo ">> Setup test resources"
ko apply -f test/config
if [[ $(kubectl get secret server-certs -n "${TEST_NAMESPACE}" -o name | wc -l) -eq 1 ]]; then
  echo ">> Enable tls against upstream"
  ko apply -f test/config/tls
  export "UPSTREAM_TLS_CERT=server-certs"
  export "UPSTREAM_CA_CERT=server-ca"
  # Use OpenSSL subjectAltName/serverName to enable the certificate for various
  # application URLs with this pattern: <APP>.<NAMESPACE>.svc.X.X
  export "SERVER_NAME=data-plane.knative.dev"
fi

IPS=($(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}'))

export "GATEWAY_OVERRIDE=kourier"
export "GATEWAY_NAMESPACE_OVERRIDE=${KOURIER_GATEWAY_NAMESPACE}"

echo ">> Running conformance tests"
go test -count=1 -short -timeout=20m -tags=e2e ./test/conformance/... ./test/e2e/... \
  --enable-alpha --enable-beta \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Scale up components for HA tests"
kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" scale deployment 3scale-kourier-gateway --replicas=2
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" scale deployment net-kourier-controller --replicas=2

echo ">> Running HA tests"
go test -count=1 -timeout=15m -failfast -parallel=1 -tags=e2e ./test/ha -spoofinterval="10ms" \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Scale down after HA tests"
kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" scale deployment 3scale-kourier-gateway --replicas=1
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" scale deployment net-kourier-controller --replicas=1

echo ">> Running TLS Cipher suites"
echo ">> Setup cipher suites"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" patch configmap/config-kourier --type merge -p '{"data":{"cipher-suites":"ECDHE-ECDSA-AES128-GCM-SHA256,ECDHE-ECDSA-CHACHA20-POLY1305"}}'

go test -v -tags=e2e ./test/tls/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

kubectl -n "${KOURIER_CONTROL_NAMESPACE}" patch configmap/config-kourier --type merge -p '{"data":{"cipher-suites":""}}'

echo ">> Setup one certificate"
$(dirname $0)/generate-cert.sh
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller CERTS_SECRET_NAMESPACE="${KOURIER_CONTROL_NAMESPACE}" CERTS_SECRET_NAME=wildcard-certs
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running OneTLSCert tests"
go test -race -count=1 -timeout=20m -tags=e2e ./test/cert/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

export "KOURIER_EXTAUTHZ_PROTOCOL=grpc"

echo ">> Setup ExtAuthz gRPC"
ko apply -f test/config/extauthz/grpc
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/externalauthz-grpc
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller KOURIER_EXTAUTHZ_HOST=externalauthz-grpc.knative-serving:6000
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running ExtAuthz tests"
go test -race -count=1 -timeout=20m -tags=e2e ./test/extauthz/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Unset ExtAuthz gRPC"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller KOURIER_EXTAUTHZ_HOST-
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller

echo ">> Setup ExtAuthz gRPC with pack as bytes option"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller \
  KOURIER_EXTAUTHZ_HOST=externalauthz-grpc.knative-serving:6000 \
  KOURIER_EXTAUTHZ_PACKASBYTES=true

kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running ExtAuthz tests"
go test -race -count=1 -timeout=20m -tags=e2e ./test/extauthz/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Unset ExtAuthz gRPC"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller KOURIER_EXTAUTHZ_HOST- KOURIER_EXTAUTHZ_PACKASBYTES-
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller

echo ">> Setup ExtAuthz HTTP"
ko apply -f test/config/extauthz/http
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/externalauthz-http
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller \
  KOURIER_EXTAUTHZ_HOST=externalauthz-http.knative-serving:8080 \
  KOURIER_EXTAUTHZ_PROTOCOL=http
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running ExtAuthz tests"
go test -race -count=1 -timeout=20m -tags=e2e ./test/extauthz/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Unset ExtAuthz HTTP"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller KOURIER_EXTAUTHZ_HOST- KOURIER_EXTAUTHZ_PROTOCOL-
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller

echo ">> Setup ExtAuthz HTTP with path prefix"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment externalauthz-http PATH_PREFIX="/check"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/externalauthz-http
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller \
  KOURIER_EXTAUTHZ_HOST=externalauthz-http.knative-serving:8080 \
  KOURIER_EXTAUTHZ_PROTOCOL=http \
  KOURIER_EXTAUTHZ_PATHPREFIX="/check"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running ExtAuthz tests"
go test -race -count=1 -timeout=20m -tags=e2e ./test/extauthz/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Unset ExtAuthz HTTP with path prefix"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" set env deployment net-kourier-controller KOURIER_EXTAUTHZ_HOST- KOURIER_EXTAUTHZ_PROTOCOL- KOURIER_EXTAUTHZ_PATHPREFIX-
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller

echo ">> Setup Proxy Protocol"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" patch configmap/config-kourier --type merge -p '{"data":{"enable-proxy-protocol":"true"}}'
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout restart deployment/net-kourier-controller
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running Proxy Protocol tests"
go test -race -count=1 -timeout=5m -tags=e2e ./test/proxyprotocol/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Unset Proxy Protocol"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" patch configmap/config-kourier --type merge -p '{"data":{"enable-proxy-protocol":"false"}}'
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout restart deployment/net-kourier-controller
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Setup Tracing"
ko apply -f test/config/tracing
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployment/tracing-backend-server
export TRACING_COLLECTOR_HOST="$(kubectl -n "${KOURIER_CONTROL_NAMESPACE}" get svc/tracing-backend-server -o jsonpath='{.spec.clusterIP}')"
export TRACING_COLLECTOR_PORT="9411"
export TRACING_COLLECTOR_ENDPOINT="/api/v2/spans"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" patch configmap/config-kourier --type merge -p "{
  \"data\":{
    \"tracing-enabled\": \"true\",
    \"tracing-collector-host\": \"$TRACING_COLLECTOR_HOST\",
    \"tracing-collector-port\": \"$TRACING_COLLECTOR_PORT\",
    \"tracing-collector-endpoint\": \"$TRACING_COLLECTOR_ENDPOINT\"
  }
}"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout restart deployment/net-kourier-controller
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running Tracing tests"
go test -race -count=1 -timeout=5m -tags=e2e ./test/tracing/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"

echo ">> Unset Tracing"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" patch configmap/config-kourier --type merge -p '{"data":{"tracing-enabled": "false"}}'
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout restart deployment/net-kourier-controller
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s
ko delete -f test/config/tracing

echo ">> Set IdleTimeout to 50s"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" patch configmap/config-kourier --type merge -p '{"data":{"stream-idle-timeout":"50s"}}'
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout restart deployment/net-kourier-controller
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" rollout status deployment/net-kourier-controller --timeout=300s

echo ">> Running IdleTimeout tests"
go test -v  -tags=e2e ./test/timeout/... \
  --ingressendpoint="${IPS[0]}" \
  --ingressClass=kourier.ingress.networking.knative.dev \
  --cluster-suffix="$CLUSTER_SUFFIX"
