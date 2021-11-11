# Kourier

**[This component is GA](https://github.com/knative/community/tree/main/mechanics/MATURITY-LEVELS.md)**

Kourier is an Ingress for [Knative Serving](https://knative.dev/). Kourier is a
lightweight alternative for the Istio ingress as its deployment consists only of
an Envoy proxy and a control plane for it.

Kourier is passing the knative serving e2e and conformance tests:
[Kourier Testgrid](https://testgrid.knative.dev/serving#kourier-stable).

- [**Getting started**](#getting-started)
- [**Features**](#features)
- [**Deployment**](#deployment)
- [**Development**](#development)
- [**License**](#license)

## Getting started

- Install Knative Serving, ideally without Istio:

```bash
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/latest/download/serving-core.yaml
```

- Then install Kourier:

```bash
kubectl apply -f https://github.com/knative/net-kourier/releases/latest/download/kourier.yaml
```

- Configure Knative Serving to use the proper "ingress.class":

```bash
kubectl patch configmap/config-network \
  -n knative-serving \
  --type merge \
  -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
```

- (OPTIONAL) Set your desired domain (replace 127.0.0.1.nip.io to your preferred
  domain):

```bash
kubectl patch configmap/config-domain \
  -n knative-serving \
  --type merge \
  -p '{"data":{"127.0.0.1.nip.io":""}}'
```

- (OPTIONAL) Deploy a sample hello world app:

```bash
cat <<-EOF | kubectl apply -f -
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: helloworld-go
spec:
  template:
    spec:
      containers:
      - image: gcr.io/knative-samples/helloworld-go
        env:
        - name: TARGET
          value: Go Sample v1
EOF
```

- (OPTIONAL) For testing purposes, you can use port-forwarding to make requests
  to Kourier from your machine:

```bash
kubectl port-forward --namespace kourier-system $(kubectl get pod -n kourier-system -l "app=3scale-kourier-gateway" --output=jsonpath="{.items[0].metadata.name}") 8080:8080 19000:19000 8443:8443

curl -v -H "Host: helloworld-go.default.127.0.0.1.nip.io" http://localhost:8080
```

## Deployment

By default, the deployment of the Kourier components is split between two
different namespaces:

- Kourier control is deployed in the `knative-serving` namespace
- The kourier gateways are deployed in the `kourier-system` namespace

To change the Kourier gateway namespace, you will need to:

- Modify the files in `config/` and replace all the namespaces fields that have
  `kourier-system` with the desired namespace.
- Set the `KOURIER_GATEWAY_NAMESPACE` env var in the kourier-control deployment
  to the new namespace.

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

If you want to enable the external authorization support you can set these ENV
vars in the `net-kourier-controller` deployment:

- `KOURIER_EXTAUTHZ_HOST*`: The external authorization service and port,
  my-auth:2222
- `KOURIER_EXTAUTHZ_FAILUREMODEALLOW*`: Allow traffic to go through if the ext
  auth service is down. Accepts true/false
- `KOURIER_EXTAUTHZ_MAXREQUESTBYTES`: Max request bytes, if not set, defaults to
  8192 Bytes. More info
  [Envoy Docs](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_authz/v3/ext_authz.proto.html?highlight=max_request_bytes#extensions-filters-http-ext-authz-v3-buffersettings)
- `KOURIER_EXTAUTHZ_TIMEOUT`: Max time in ms to wait for the ext authz service.
  Defaults to 2s.

`*` Required

- Run `make help` for the complete list of make targets available.

## License

[Apache 2.0 License](LICENSE)
