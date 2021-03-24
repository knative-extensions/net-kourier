module knative.dev/net-kourier

go 1.15

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20201210154907-fd9021fe5dad
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.2.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	go.uber.org/zap v1.16.0
	google.golang.org/genproto v0.0.0-20201214200347-8c77b98c765d
	google.golang.org/grpc v1.36.0
	google.golang.org/protobuf v1.25.0
	gotest.tools/v3 v3.0.3
	k8s.io/api v0.19.7
	k8s.io/apimachinery v0.19.7
	k8s.io/client-go v0.19.7
	knative.dev/hack v0.0.0-20210317214554-58edbdc42966
	knative.dev/networking v0.0.0-20210323181619-8fc278deb519
	knative.dev/pkg v0.0.0-20210323202917-b558677ab034
)
