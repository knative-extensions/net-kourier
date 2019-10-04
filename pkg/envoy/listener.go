package envoy

import (
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	httpconnmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"os"
)

const (
	envCertsSecretNamespace = "CERTS_SECRET_NAMESPACE"
	envCertsSecretName      = "CERTS_SECRET_NAME"
	certFieldInSecret       = "tls.crt"
	keyFieldInSecret        = "tls.key"
	httpPort                = uint32(8080)
	httpsPort               = uint32(8443)
)

func newEnvoyListener(https bool,
	manager *httpconnmanagerv2.HttpConnectionManager,
	kubeClient KubeClient) (*v2.Listener, error) {

	if https {
		return envoyHTTPSListener(manager, kubeClient)
	} else {
		return envoyHTTPListener(manager)
	}
}

func envoyHTTPListener(manager *httpconnmanagerv2.HttpConnectionManager) (*v2.Listener, error) {
	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	envoyListener := &v2.Listener{
		Name:    "listener_0",
		Address: createAddress(httpPort),
		FilterChains: []*listener.FilterChain{
			{
				Filters: filters,
			},
		},
	}

	return envoyListener, nil
}

func envoyHTTPSListener(manager *httpconnmanagerv2.HttpConnectionManager,
	kubeClient KubeClient) (*v2.Listener, error) {

	secret, err := kubeClient.GetSecret(os.Getenv(envCertsSecretNamespace),
		os.Getenv(envCertsSecretName))
	if err != nil {
		return nil, err
	}

	certificateChain := string(secret.Data[certFieldInSecret])
	privateKey := string(secret.Data[keyFieldInSecret])

	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	envoyListener := v2.Listener{
		Name:    "listener_0",
		Address: createAddress(httpsPort),
		FilterChains: []*listener.FilterChain{
			{
				TlsContext: createTLSContext(certificateChain, privateKey),
				Filters:    filters,
			},
		},
	}

	return &envoyListener, nil
}

func createAddress(port uint32) *core.Address {
	return &core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  "0.0.0.0",
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func createFilters(manager *httpconnmanagerv2.HttpConnectionManager) ([]*listener.Filter, error) {
	pbst, err := util.MessageToStruct(manager)
	if err != nil {
		return []*listener.Filter{}, err
	}

	filters := []*listener.Filter{
		{
			Name:       util.HTTPConnectionManager,
			ConfigType: &listener.Filter_Config{Config: pbst},
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
