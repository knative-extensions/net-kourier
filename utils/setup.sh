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
export KO_DOCKER_REPO=kind.local
ko resolve -f config | \
  sed 's/LoadBalancer/NodePort/g' | \
  kubectl apply -f -

echo "Wait for all deployments to be up"
kubectl -n "${KOURIER_CONTROL_NAMESPACE}" wait --timeout=300s --for=condition=Available deployments --all
kubectl -n "${KOURIER_GATEWAY_NAMESPACE}" wait --timeout=300s --for=condition=Available deployments --all

ip=$(kubectl get nodes -lkubernetes.io/hostname!=kind-control-plane -ojsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}' | head -n1)
port=$(kubectl -n kourier-system get svc kourier -ojsonpath='{.spec.ports[?(@.name=="http2")].nodePort}')
echo
echo "You can connect to Kourier at ${ip}:${port}"
echo "Consider exporting it via 'export KOURIER_IP=${ip}:${port}'"
echo "Example usage: 'curl -H \"Host: helloworld-go.default.example.com\" \$KOURIER_IP'"
