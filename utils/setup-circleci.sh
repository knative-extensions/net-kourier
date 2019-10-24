#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

if ! command -v microk8s.kubectl >/dev/null; then
  echo "You need to install microk8s"
  exit 1
fi

tag="test_$(git rev-parse --abbrev-ref HEAD)"
microk8s.kubectl apply -f https://github.com/knative/serving/releases/download/v0.9.0/serving-core.yaml || true
mkdir -p "$HOME"/.kube/
microk8s.kubectl config view --raw > "$HOME"/.kube/config
chown -R circleci "$HOME"/.kube

docker build -t 3scale-kourier:"$tag" ./
docker image save 3scale-kourier:"$tag" > image.tar
microk8s.ctr -n k8s.io images import image.tar
microk8s.enable dns
microk8s.kubectl apply -f deploy/kourier-knative.yaml
microk8s.kubectl patch deployment 3scale-kourier-control -n knative-serving --patch "{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"kourier-control\",\"image\": \"3scale-kourier:$tag\",\"imagePullPolicy\": \"IfNotPresent\"}]}}}}"
microk8s.kubectl patch configmap/config-domain -n knative-serving --type merge -p '{"data":{"127.0.0.1.nip.io":""}}'
microk8s.kubectl patch configmap/config-network -n knative-serving --type merge -p '{"data":{"clusteringress.class":"kourier.ingress.networking.knative.dev","ingress.class":"kourier.ingress.networking.knative.dev"}}'

retries=0
while [[ $(microk8s.kubectl get pods -n knative-serving -l app=3scale-kourier-control -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier control pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timed out waiting for kourier control pod"
    exit 1
  fi
  retries=$[$retries+1]
done

retries=0
while [[ $(microk8s.kubectl get pods -n knative-serving -l app=3scale-kourier-gateway -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier gateway pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timed out waiting for kourier gateway pod"
    exit 1
  fi
  retries=$[$retries+1]
done

microk8s.kubectl port-forward --namespace knative-serving $(microk8s.kubectl get pod -n knative-serving -l "app=3scale-kourier-gateway" --output=jsonpath="{.items[0].metadata.name}") 8080:8080 19000:19000 &>/dev/null &