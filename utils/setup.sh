#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

if ! command -v k3d >/dev/null; then
  echo "k3d binary not in path, install with: curl -s https://raw.githubusercontent.com/rancher/k3d/master/install.sh | bash"
  exit 1
fi

tag="test_$(git rev-parse --abbrev-ref HEAD)"

# In CircleCI, PR branches that come from forks have the format "pull/n", where
# n is the PR number. "/" is not accepted in docker tags, so we need to replace
# it.
tag=$(echo "$tag" | tr / -)

k3d d --name=kourier-integration || true

k3d c --name kourier-integration --server-arg "--no-deploy=traefik"
sleep 60
export KUBECONFIG="$(k3d get-kubeconfig --name='kourier-integration')"

# Builds and imports the kourier and gateway images from docker into the k8s cluster
docker build -t 3scale-kourier:"$tag" ./
docker build -f Dockerfile.gateway -t 3scale-kourier-gateway:"$tag" ./
k3d import-images 3scale-kourier:"$tag" --name='kourier-integration'
k3d import-images 3scale-kourier-gateway:"$tag" --name='kourier-integration'

KNATIVE_VERSION=v0.10.0
# Deploys kourier and patches it.
kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl apply -f deploy/kourier-knative.yaml
kubectl patch deployment 3scale-kourier-control -n knative-serving --patch "{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"kourier-control\",\"image\": \"3scale-kourier:$tag\",\"imagePullPolicy\": \"IfNotPresent\"}]}}}}"
kubectl patch deployment 3scale-kourier-gateway -n knative-serving --patch "{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"kourier-gateway\",\"image\": \"3scale-kourier-gateway:$tag\",\"imagePullPolicy\": \"IfNotPresent\"}]}}}}"
kubectl patch configmap/config-domain -n knative-serving --type merge -p '{"data":{"127.0.0.1.nip.io":""}}'
kubectl patch configmap/config-network -n knative-serving --type merge -p '{"data":{"clusteringress.class":"kourier.ingress.networking.knative.dev","ingress.class":"kourier.ingress.networking.knative.dev"}}'

retries=0
while [[ $(kubectl get pods -n knative-serving -l app=3scale-kourier-control -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier control pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timedout waiting for kourier control pod"
    exit 1
  fi
  retries=$((retries + 1))
done

retries=0
while [[ $(kubectl get pods -n knative-serving -l app=3scale-kourier-gateway -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier gateway pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timedout waiting for kourier gateway pod"
    exit 1
  fi
  retries=$((retries + 1))
done

# shellcheck disable=SC2046
kubectl port-forward --namespace knative-serving "$(kubectl get pod -n knative-serving -l "app=3scale-kourier-gateway" --output=jsonpath="{.items[0].metadata.name}")" 8080:8080 8081:8081 19000:19000 8443:8443 &>/dev/null &
