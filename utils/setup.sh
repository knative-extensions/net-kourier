#!/bin/bash

source $(dirname $0)/../test/e2e-common.sh

set -euo pipefail
IFS=$'\n\t'

KNATIVE_NAMESPACE=knative-serving
KOURIER_GATEWAY_NAMESPACE=kourier-system
KOURIER_CONTROL_NAMESPACE=${KNATIVE_NAMESPACE}
if ! command -v k3d >/dev/null; then
  echo "k3d binary not in path, install with: curl -s https://raw.githubusercontent.com/rancher/k3d/master/install.sh | bash"
  exit 1
fi

tag="test_$(git rev-parse --abbrev-ref HEAD)"

# In CircleCI, PR branches that come from forks have the format "pull/n", where
# n is the PR number. "/" is not accepted in docker tags, so we need to replace
# it.
tag=$(echo "$tag" | tr / -)

k3d cluster delete kourier-integration || true
k3d cluster create --wait --no-lb --k3s-server-arg '--no-deploy=traefik' kourier-integration

# Builds and imports the kourier and gateway images from docker into the k8s cluster
docker build -t 3scale-kourier:"$tag" ./
docker build -f ./utils/extauthz_test_image/Dockerfile -t test_externalauthz:test ./utils/extauthz_test_image/
k3d image import 3scale-kourier:"$tag" -c 'kourier-integration'
k3d image import test_externalauthz:test -c 'kourier-integration'

KNATIVE_VERSION=v0.17.0
# Deploys kourier and patches it.
kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl apply -f deploy/kourier-knative.yaml
kubectl patch configmap/config-domain -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"127.0.0.1.nip.io":""}}'
kubectl patch configmap/config-network -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
retries=0
while [[ $(kubectl get pods -n ${KOURIER_CONTROL_NAMESPACE} -l app=3scale-kourier-control -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier control pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timedout waiting for kourier control pod"
    exit 1
  fi
  retries=$((retries + 1))
done

retries=0
while [[ $(kubectl get pods -n ${KOURIER_GATEWAY_NAMESPACE} -l app=3scale-kourier-gateway -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier gateway pod to be ready "
  sleep 10
  if [ $retries -ge 10 ]; then
    echo "timedout waiting for kourier gateway pod"
    exit 1
  fi
  retries=$((retries + 1))
done

# shellcheck disable=SC2046
kubectl port-forward --namespace ${KOURIER_GATEWAY_NAMESPACE} "$(kubectl get pod -n ${KOURIER_GATEWAY_NAMESPACE} -l "app=3scale-kourier-gateway" --output=jsonpath="{.items[0].metadata.name}")" 8080:8080 8081:8081 19000:19000 8443:8443 &>/dev/null &
