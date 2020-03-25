#!/usr/bin/env bash

set -x

POD_NAME=$(kubectl get pods --selector=app=monitoring --output=jsonpath='{.items[*].metadata.name}')

echo "Building clear-alerts binary"
go build -o clearalerts

echo "Copy the clear-alerts binary to the monitoring pod ${POD_NAME}"
kubectl cp clearalerts "${POD_NAME}:/clearalerts"

echo "Executing clear-alerts on the pod"
kubectl exec ${POD_NAME} -- /clearalerts
