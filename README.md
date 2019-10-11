# Kourier

[![CircleCI](https://circleci.com/gh/3scale/kourier.svg?style=svg)](https://circleci.com/gh/3scale/kourier)

Kourier is an Ingress for [Knative](https://knative.dev/). A deployment of
Kourier consists of an Envoy proxy and a control plane for it. Kourier is meant
to be a lightweight replacement for the Istio ingress. In the future, it
will provide API management capabilities.

This project is under early development. We are working on passing all the
end-to-end tests of the Knative suite.

- [**Getting started**](#getting-started)
- [**Features**](#features)
- [**Development**](#development)
- [**License**](#license)


## Getting started

- Pre-requisites: a Kubernetes cluster with Knative serving version >= 0.8.

- To install Kourier:
```bash
kubectl -f apply deploy/kourier-knative.yaml
```

- For testing purposes, you can use port-forwarding to make requests to Kourier
from your machine:
```bash
kubectl port-forward --namespace knative-serving
$(kubectl get pod -n knative-serving -l "app=3scale-kourier"
--output=jsonpath="{.items[0].metadata.name}") 8080:8080 19000:19000 8443:8443`
```


## Features

- Traffic splitting between Knative revisions.
- Automatic update of endpoints as they are scaled.
- Support for gRPC services.
- Timeouts and retries.
- TLS


## Development

- Run the test suite:
```bash
make test
```

- Run only the unit or the integration tests:
```bash
make test-unit
make test-integration
```

- Set up a local environment with Knative running on top of [k3s](https://k3s.io/):
```bash
make local-setup
```

- Run `make help` for the complete list of make targets available.


## License

[MIT License](LICENSE)
