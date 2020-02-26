# Kourier

[![CircleCI](https://circleci.com/gh/3scale/kourier.svg?style=svg)](https://circleci.com/gh/3scale/kourier)
[![Docker Repository on Quay](https://quay.io/repository/3scale/kourier/status "Docker Repository on Quay")](https://quay.io/repository/3scale/kourier)

Kourier is an Ingress for [Knative](https://knative.dev/). A deployment of
Kourier consists of an Envoy proxy and a control plane for it. Kourier is meant
to be a lightweight replacement for the Istio ingress. In the future, it
will provide API management capabilities.

Kourier is passing the knative serving e2e and conformance tests: [Kourier Testgrid](https://testgrid.knative.dev/serving#kourier-stable).

- [**Getting started**](#getting-started)
- [**Features**](#features)
- [**Development**](#development)
- [**License**](#license)


## Getting started

- Install Knative Serving, ideally without Istio by using the "serving-core.yaml":
```bash 
kubectl apply -f https://github.com/knative/serving/releases/download/v0.9.0/serving-core.yaml
```

- Then install Kourier:
```bash
kubectl apply -f deploy/kourier-knative.yaml
```

- Configure Knative Serving to use the proper "ingress.class":
```bash
kubectl patch configmap/config-network \
  -n knative-serving \
  --type merge \
  -p '{"data":{"clusteringress.class":"kourier.ingress.networking.knative.dev",
               "ingress.class":"kourier.ingress.networking.knative.dev"}}'
```

- (OPTIONAL) Set your desired domain (replace 127.0.0.1.nip.io to your prefered domain):
```bash
 kubectl patch configmap/config-domain \
  -n knative-serving \
  --type merge \
  -p '{"data":{"127.0.0.1.nip.io":""}}'
```

- (OPTIONAL) Deploy a sample hello world app:
```bash
kubectl apply -f ./samples/helloworld-go.yaml
```

- (OPTIONAL) For testing purposes, you can use port-forwarding to make requests to Kourier
from your machine:
```bash
kubectl port-forward --namespace kourier-system $(kubectl get pod -n kourier-system -l "app=3scale-kourier-gateway" --output=jsonpath="{.items[0].metadata.name}") 8080:8080 19000:19000 8443:8443

curl -v -H "Host: helloworld-go.default.127.0.0.1.nip.io" http://localhost:8080 
```

## Features

- Traffic splitting between Knative revisions.
- Automatic update of endpoints as they are scaled.
- Support for gRPC services.
- Timeouts and retries.
- TLS
- External Authorization support.

## Setup TLS certificate

Create a secret containing your TLS certificate and Private key:

```
kubectl create secret tls ${CERT_NAME} --key ${KEY_FILE} --cert ${CERT_FILE}
```

Add the following env vars to 3scale-Kourier in the "kourier" container : 

```
CERTS_SECRET_NAMESPACE: ${NAMESPACES_WHERE_THE_SECRET_HAS_BEEN_CREATED}
CERTS_SECRET_NAME: ${CERT_NAME}
```
## External Authorization Configuration

If you want to enable the external authorization support you can set these ENV vars in the `3scale-kourier-control`
 deployment:

- `KOURIER_EXTAUTHZ_HOST*`: The external authorization service and port, my-auth:2222
- `KOURIER_EXTAUTHZ_FAILUREMODEALLOW*`: Allow traffic to go through if the ext auth service is down. Accepts true/false
- `KOURIER_EXTAUTHZ_MAXREQUESTBYTES`: Max request bytes, if not set, defaults to 8192 Bytes. More info [Envoy Docs](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_authz/v3/ext_authz.proto.html?highlight=max_request_bytes#extensions-filters-http-ext-authz-v3-buffersettings)
- `KOURIER_EXTAUTHZ_TIMEOUT`: Max time in ms to wait for the ext authz service. Defaults to 2s.

 `*` Required
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

[Apache 2.0 License](LICENSE)
