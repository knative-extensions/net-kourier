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
kubectl patch configmap/config-network -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'

echo "Deploying Kourier"
export KO_DOCKER_REPO
ko resolve -f test/config -f deploy/kourier-knative.yaml | \
  sed 's/LoadBalancer/NodePort/g' | \
  kubectl apply -f -

echo "Wait for all deployments to be up"
for d in $(kubectl -n ${KOURIER_CONTROL_NAMESPACE} get deploy -oname)
do
  kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available "$d"
done

for d in $(kubectl -n ${KOURIER_GATEWAY_NAMESPACE} get deploy -oname)
do
  kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" wait --timeout=300s --for=condition=Available "$d"
done
