# Knative end-to-end tests

This document describes how to pass the Knative Serving end-to-end tests with
Kourier.

Please note that this is a work in progress and we will try to simplify the
steps detailed here.

## Prepare the Kourier environment

- (OPTIONAL) Deploy a local Kubernetes test environment with Knative Serving and
  Kourier if needed:

```bash
make local-setup
```

- We need to export a couple of envs to indicate that we are using Kourier as
  the ingress:

```bash
export GATEWAY_OVERRIDE=kourier
export GATEWAY_NAMESPACE_OVERRIDE=kourier-system
```

## Run the Knative tests

- Clone the [Knative Serving repo](https://github.com/knative/serving) and
  switch to the `v0.9.0` tag.

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

Depending on your setup, there are some tests where
`--ingress-endpoint=127.0.0.1` is needed.

When running the conformance tests, we need to set the `ingressClass` flag:

```bash
go test -v -tags=e2e ./test/conformance/ingress/ -run "^TestBasics\$" --dockerrepo "YOUR_REPO" --kubeconfig="$HOME/.kube/config" --ingressClass="kourier.ingress.networking.knative.dev"
```
