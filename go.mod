module knative.dev/net-kourier

go 1.14

require (
	github.com/envoyproxy/go-control-plane v0.9.4
	github.com/golang/protobuf v1.4.2
	github.com/google/go-cmp v0.5.2
	github.com/google/uuid v1.1.1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	go.uber.org/zap v1.15.0
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a // indirect
	google.golang.org/grpc v1.31.1
	google.golang.org/protobuf v1.25.0
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/networking v0.0.0-20200922180040-a71b40c69b15
	knative.dev/pkg v0.0.0-20200922214939-58028584ae4b
	knative.dev/serving v0.17.1-0.20200923102239-134e55230265
	knative.dev/test-infra v0.0.0-20200923011439-4ef663a0aa96
)

replace (
	github.com/envoyproxy/go-control-plane => github.com/envoyproxy/go-control-plane v0.9.1
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	github.com/tsenart/vegeta => github.com/tsenart/vegeta v1.2.1-0.20190917092155-ab06ddb56e2f

	k8s.io/api => k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.8
	k8s.io/client-go => k8s.io/client-go v0.18.8
	k8s.io/code-generator => k8s.io/code-generator v0.18.8
)

// TODO(nak3): DO NOT SUBMIT
replace knative.dev/networking => github.com/nak3/networking v0.0.0-20200923141347-aca1b6a6097f
