module knative.dev/net-kourier

go 1.15

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20210217033140-668b12f5399d
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.2.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	go.uber.org/zap v1.16.0
	google.golang.org/genproto v0.0.0-20210416161957-9910b6c460de
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	gotest.tools/v3 v3.0.3
	k8s.io/api v0.19.7
	k8s.io/apimachinery v0.19.7
	k8s.io/client-go v0.19.7
	k8s.io/code-generator v0.19.7
	knative.dev/hack v0.0.0-20210601210329-de04b70e00d0
	knative.dev/networking v0.0.0-20210601151838-6ce35e5687a3
	knative.dev/pkg v0.0.0-20210602044730-a2869ae1dce3
)
