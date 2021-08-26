/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package envoy

import (
	"fmt"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	auth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/types"
)

// SNIMatch represents an SNI match, including the hosts to match, the certificates and
// keys to use and the source where we got the certs/keys from.
type SNIMatch struct {
	Hosts            []string
	CertSource       types.NamespacedName
	CertificateChain []byte
	PrivateKey       []byte
}

// NewHTTPListener creates a new Listener at the given port, backed by the given manager.
func NewHTTPListener(manager *hcm.HttpConnectionManager, port uint32) (*listener.Listener, error) {
	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	return &listener.Listener{
		Name:    fmt.Sprintf("listener_%d", port),
		Address: createAddress(port),
		FilterChains: []*listener.FilterChain{{
			Filters: filters,
		}},
	}, nil
}

// NewHTTPSListener creates a new Listener at the given port, backed by the given manager
// and serving the given certificate chain and key.
func NewHTTPSListener(
	manager *hcm.HttpConnectionManager,
	port uint32,
	certificateChain []byte,
	privateKey []byte) (*listener.Listener, error) {

	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	tlsContext := createTLSContext(certificateChain, privateKey)
	tlsAny, err := anypb.New(tlsContext)
	if err != nil {
		return nil, err
	}

	return &listener.Listener{
		Name:    fmt.Sprintf("listener_%d", port),
		Address: createAddress(port),
		FilterChains: []*listener.FilterChain{{
			Filters: filters,
			TransportSocket: &core.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: tlsAny},
			},
		}},
	}, nil
}

// NewHTTPSListenerWithSNI creates a new Listener at the given port, backed by the given
// manager and applies a FilterChain with the given sniMatches.
//
// Ref: https://www.envoyproxy.io/docs/envoy/latest/faq/configuration/sni.html
func NewHTTPSListenerWithSNI(manager *hcm.HttpConnectionManager, port uint32, sniMatches []*SNIMatch) (*listener.Listener, error) {
	filterChains, err := createFilterChainsForTLS(manager, sniMatches)
	if err != nil {
		return nil, err
	}

	return &listener.Listener{
		Name:         fmt.Sprintf("listener_%d", port),
		Address:      createAddress(port),
		FilterChains: filterChains,
		ListenerFilters: []*listener.ListenerFilter{{
			// TLS Inspector listener filter must be configured in order to
			// detect requested SNI.
			// Ref: https://www.envoyproxy.io/docs/envoy/latest/faq/configuration/sni.html
			Name: wellknown.TlsInspector,
		}},
	}, nil
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

func createFilters(manager *hcm.HttpConnectionManager) ([]*listener.Filter, error) {
	managerAny, err := anypb.New(manager)
	if err != nil {
		return nil, err
	}

	return []*listener.Filter{{
		Name:       wellknown.HTTPConnectionManager,
		ConfigType: &listener.Filter_TypedConfig{TypedConfig: managerAny},
	}}, nil
}

func createFilterChainsForTLS(manager *hcm.HttpConnectionManager, sniMatches []*SNIMatch) ([]*listener.FilterChain, error) {
	res := make([]*listener.FilterChain, 0, len(sniMatches))
	for _, sniMatch := range sniMatches {
		filters, err := createFilters(manager)
		if err != nil {
			return nil, err
		}

		tlsContext := createTLSContext(sniMatch.CertificateChain, sniMatch.PrivateKey)
		tlsAny, err := anypb.New(tlsContext)
		if err != nil {
			return nil, err
		}

		filterChain := listener.FilterChain{
			FilterChainMatch: &listener.FilterChainMatch{
				ServerNames: sniMatch.Hosts,
			},
			TransportSocket: &core.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: tlsAny},
			},
			Filters: filters,
		}

		res = append(res, &filterChain)
	}

	return res, nil
}

func createTLSContext(certificate []byte, privateKey []byte) *auth.DownstreamTlsContext {
	return &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			AlpnProtocols: []string{"h2", "http/1.1"},
			TlsCertificates: []*auth.TlsCertificate{{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{InlineBytes: certificate},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{InlineBytes: privateKey},
				},
			}},
		},
	}
}
