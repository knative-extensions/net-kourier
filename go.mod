module knative.dev/net-kourier

go 1.16

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20210512163311-63b5d3c536b0
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/grpc-gateway v1.14.8 // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	go.uber.org/zap v1.18.1
	google.golang.org/genproto v0.0.0-20210713002101-d411969a0d9a
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
	gotest.tools/v3 v3.0.3
	k8s.io/api v0.20.7
	k8s.io/apimachinery v0.20.7
	k8s.io/client-go v0.20.7
	k8s.io/code-generator v0.20.7
	knative.dev/hack v0.0.0-20210806075220-815cd312d65c
	knative.dev/networking v0.0.0-20210824140523-51512a042e23
	knative.dev/pkg v0.0.0-20210824120823-a94f5f07b3c3
)

replace github.com/envoyproxy/go-control-plane => github.com/envoyproxy/go-control-plane v0.9.9-0.20210217033140-668b12f5399d
