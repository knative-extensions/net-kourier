# Knative end-to-end tests

This document describes how to pass the Knative Serving end-to-end tests with
Kourier.

Please note that this is a work in progress and we will try to simplify the
steps detailed here.

## Prepare the Kourier environment

- (OPTIONAL) Deploy a local Kubernetes test environment with Knative Serving and
  Kourier if needed:

- We need to export a couple of envs to indicate that we are using Kourier as
  the ingress:

```bash
export GATEWAY_OVERRIDE=kourier
export GATEWAY_NAMESPACE_OVERRIDE=kourier-system
```

## Run the Knative tests

- Clone the [Knative Serving repo](https://github.com/knative/serving) and
  switch to the `knative-v1.17.0` tag.

- As stated in the Knative instructions, you'll need to
  [build some test images](https://github.com/knative/serving/blob/main/test/README.md#test-images)
  and create some resources in your cluster:

```bash
KO_DOCKER_REPO=YOUR_REPO ko apply -f test/config/100-namespace.yaml
```

- Now you can run tests individually. For example, to execute the
  `CallToPublicService` test, run:

```bash
go test -v  -tags=e2e ./test/e2e -run "^TestCallToPublicService" --dockerrepo "YOUR_REPO" --kubeconfig="$HOME/.kube/config"
```
