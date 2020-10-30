#!/bin/bash

set -euo pipefail

KNATIVE_NAMESPACE=knative-serving
KOURIER_GATEWAY_NAMESPACE=kourier-system
KOURIER_CONTROL_NAMESPACE=${KNATIVE_NAMESPACE}

export KIND_CLUSTER_NAME="kourier-integration"
kind delete cluster
kind create cluster

echo "Deploying Knative Serving"
KNATIVE_VERSION=v0.18.0
kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl patch configmap/config-domain -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"127.0.0.1.nip.io":""}}'
kubectl patch configmap/config-network -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'

echo "Deploying Kourier"
KO_DOCKER_REPO=kind.local ko apply -Rf deploy/kourier-knative.yaml

echo "Wait for all deployments to be up"
for d in $(kubectl -n ${KOURIER_CONTROL_NAMESPACE} get deploy -oname)
do
  kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available "$d"
done

for d in $(kubectl -n ${KOURIER_GATEWAY_NAMESPACE} get deploy -oname)
do
  kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" wait --timeout=300s --for=condition=Available "$d"
done

# shellcheck disable=SC2046
kubectl port-forward --namespace ${KOURIER_GATEWAY_NAMESPACE} "$(kubectl get pod -n ${KOURIER_GATEWAY_NAMESPACE} -l "app=3scale-kourier-gateway" --output=jsonpath="{.items[0].metadata.name}")" 8080:8080 8081:8081 19000:19000 8443:8443 &>/dev/null &
