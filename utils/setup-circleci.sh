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
microk8s.kubectl apply -f deploy/kourier-knative.yaml
microk8s.kubectl patch deployment 3scale-kourier -n knative-serving --patch "{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"kourier\",\"image\": \"3scale-kourier:$tag\",\"imagePullPolicy\": \"IfNotPresent\"}]}}}}"

retries=0
while [[ $(microk8s.kubectl get pods -n knative-serving -l app=3scale-kourier -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timedout waiting for kourier pod"
    exit 1
  fi
  retries=$[$retries+1]
done


microk8s.kubectl port-forward --namespace knative-serving $(microk8s.kubectl get pod -n knative-serving -l "app=3scale-kourier" --output=jsonpath="{.items[0].metadata.name}") 8080:8080 19000:19000 &>/dev/null &