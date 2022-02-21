module knative.dev/net-kourier

go 1.16

require (
	github.com/envoyproxy/go-control-plane v0.10.1
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.3.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pires/go-proxyproto v0.6.1
	go.uber.org/zap v1.19.1
	golang.org/x/sys v0.0.0-20220110181412-a018aaa089fe // indirect
	google.golang.org/genproto v0.0.0-20211129164237-f09f9a12af12
	google.golang.org/grpc v1.42.0
	google.golang.org/protobuf v1.27.1
	gotest.tools/v3 v3.0.3
	k8s.io/api v0.22.5
	k8s.io/apimachinery v0.22.5
	k8s.io/client-go v0.22.5
	k8s.io/code-generator v0.22.5
	knative.dev/hack v0.0.0-20220218190734-a8ef7b67feec
	knative.dev/networking v0.0.0-20220218172835-17e1877df34f
	knative.dev/pkg v0.0.0-20220217155112-d48172451966
)
