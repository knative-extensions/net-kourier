package envoy

import (
	"fmt"
	"kourier/pkg/config"
	"os"

	"github.com/golang/protobuf/ptypes"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	httpconnmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

const (
	envCertsSecretNamespace = "CERTS_SECRET_NAMESPACE"
	envCertsSecretName      = "CERTS_SECRET_NAME"
	certFieldInSecret       = "tls.crt"
	keyFieldInSecret        = "tls.key"
)

func newExternalEnvoyListener(https bool,
	manager *httpconnmanagerv2.HttpConnectionManager,
	kubeClient kubeclient.Interface) (*v2.Listener, error) {

	if https {
		return envoyHTTPSListener(manager, kubeClient, config.HttpsPortExternal)
	} else {
		return envoyHTTPListener(manager, config.HttpPortExternal)
	}
}

func newInternalEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager) (*v2.Listener, error) {
	return envoyHTTPListener(manager, config.HttpPortInternal)
}

func envoyHTTPListener(manager *httpconnmanagerv2.HttpConnectionManager, port uint32) (*v2.Listener, error) {
	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	envoyListener := &v2.Listener{
		Name:    fmt.Sprintf("listener_%d", port),
		Address: createAddress(port),
		FilterChains: []*listener.FilterChain{
			{
				Filters: filters,
			},
		},
	}

	return envoyListener, nil
}

func envoyHTTPSListener(manager *httpconnmanagerv2.HttpConnectionManager,
	kubeClient kubeclient.Interface,
	port uint32) (*v2.Listener, error) {

	secret, err := kubeClient.CoreV1().Secrets(os.Getenv(envCertsSecretNamespace)).Get(
		os.Getenv(envCertsSecretName), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	certificateChain := string(secret.Data[certFieldInSecret])
	privateKey := string(secret.Data[keyFieldInSecret])

	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	tlsContext := createTLSContext(certificateChain, privateKey)
	tlsAny, err := ptypes.MarshalAny(tlsContext)
	if err != nil {
		return nil, err
	}

	envoyListener := v2.Listener{
		Name:    fmt.Sprintf("listener_%d", port),
		Address: createAddress(port),
		FilterChains: []*listener.FilterChain{
			{
				Filters: filters,
				TransportSocket: &core.TransportSocket{
					Name:       "tls",
					ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: tlsAny},
				},
				Name: "tlsContext",
			},
		},
	}

	return &envoyListener, nil
}

func createAddress(port uint32) *core.Address {
	return &core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.SocketAddress_TCP,
				Address:  "0.0.0.0",
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func createFilters(manager *httpconnmanagerv2.HttpConnectionManager) ([]*listener.Filter, error) {
	managerAny, err := ptypes.MarshalAny(manager)
	if err != nil {
		return []*listener.Filter{}, err
	}

	filters := []*listener.Filter{
		{
			Name:       wellknown.HTTPConnectionManager,
			ConfigType: &listener.Filter_TypedConfig{TypedConfig: managerAny},
		},
	}

	return filters, nil
}

func createTLSContext(certificate string, privateKey string) *auth.DownstreamTlsContext {
	return &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsCertificates: []*auth.TlsCertificate{
				{
					CertificateChain: &core.DataSource{
						Specifier: &core.DataSource_InlineBytes{InlineBytes: []byte(certificate)},
					},
					PrivateKey: &core.DataSource{
						Specifier: &core.DataSource_InlineBytes{InlineBytes: []byte(privateKey)},
					},
				},
			},
		},
	}
}
