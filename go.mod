module knative.dev/net-kourier

go 1.14

require (
	github.com/envoyproxy/go-control-plane v0.9.8
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.4
	github.com/google/uuid v1.1.2
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	go.uber.org/zap v1.16.0
	google.golang.org/genproto v0.0.0-20201211151036-40ec1c210f7a
	google.golang.org/grpc v1.34.0
	google.golang.org/protobuf v1.25.0
	gotest.tools/v3 v3.0.3
	k8s.io/api v0.18.12
	k8s.io/apimachinery v0.18.12
	k8s.io/client-go v0.18.12
	knative.dev/hack v0.0.0-20210114150620-4422dcadb3c8
	knative.dev/networking v0.0.0-20210115031420-356d71b7eee6
	knative.dev/pkg v0.0.0-20210114223020-f0ea5e6b9c4e
)
