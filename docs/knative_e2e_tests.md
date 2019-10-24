# Knative end-to-end tests

This document describes how to pass the Knative Serving end-to-end tests with
Kourier.

Please note that this is a work in progress and we will try to simplify the
steps detailed here.

## Prepare the Kourier environment

- (OPTIONAL) Deploy a local Kubernetes test environment with Knative Serving and Kourier if needed:
```bash
make local-setup
```

- We need a slightly different environment because the Knative tests assume
certain namespaces, service names, etc. So we need to change a few things:
```bash
kubectl apply -f deploy/knative_e2e_tests/kourier-istio-system.yaml
```

- Build the image that you want to try, upload it to the cluster and patch the
deployment to use it:
```bash
docker build -t 3scale-kourier:my_tests ./

k3d import-images 3scale-kourier:my_tests --name='kourier-integration'

kubectl patch deployment 3scale-kourier -n istio-system --patch "{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"kourier\",\"image\": \"3scale-kourier:my_tests\",\"imagePullPolicy\": \"IfNotPresent\"}]}}}}"
```

- Clean-up some things that are not needed:
```bash
kubectl delete deployment 3scale-kourier -n knative-serving
kubectl delete service kourier -n knative-serving
kubectl delete service traefik -n kube-system
```

- Edit the config domain adding `127.0.0.1.nip.io: ""` just below `data:`
```bash
kubectl edit configmap config-domain -n knative-serving
```

- Port-forward Kourier. Make sure you do not have anything else on these ports,
otherwise, it will fail:
```bash
sudo kubectl port-forward --namespace istio-system $(kubectl get pod -n istio-system -l "app=3scale-kourier" --output=jsonpath="{.items[0].metadata.name}") 80:8080 8081:8081 19000:19000 8443:8443
```

## Run the Knative tests

- Clone the [Knative Serving repo](https://github.com/knative/serving) and switch
to the `v0.9.0` tag.

- As stated in the Knative instructions, you'll need to [build some test
images](https://github.com/knative/serving/blob/master/test/README.md#test-images)
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
