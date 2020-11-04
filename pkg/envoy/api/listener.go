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

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	httpconnmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
)

type SNIMatch struct {
	hosts            []string
	certificateChain []byte
	privateKey       []byte
}

func NewSNIMatch(hosts []string, certificateChain []byte, privateKey []byte) SNIMatch {
	return SNIMatch{
		hosts:            hosts,
		certificateChain: certificateChain,
		privateKey:       privateKey,
	}
}

func NewHTTPListener(manager *httpconnmanagerv2.HttpConnectionManager, port uint32) (*v2.Listener, error) {
	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	return &v2.Listener{
		Name:    fmt.Sprintf("listener_%d", port),
		Address: createAddress(port),
		FilterChains: []*listener.FilterChain{{
			Filters: filters,
		}},
	}, nil
}

func NewHTTPSListener(manager *httpconnmanagerv2.HttpConnectionManager,
	port uint32,
	certificateChain []byte,
	privateKey []byte) (*v2.Listener, error) {

	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	tlsContext := createTLSContext(certificateChain, privateKey)
	tlsAny, err := ptypes.MarshalAny(tlsContext)
	if err != nil {
		return nil, err
	}

	return &v2.Listener{
		Name:    fmt.Sprintf("listener_%d", port),
		Address: createAddress(port),
		FilterChains: []*listener.FilterChain{{
			Filters: filters,
			TransportSocket: &core.TransportSocket{
				Name:       "tls",
				ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: tlsAny},
			},
		}},
	}, nil
}

// Configures a Listener with SNI.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/faq/configuration/sni.html
func NewHTTPSListenerWithSNI(manager *httpconnmanagerv2.HttpConnectionManager, port uint32, sniMatches []*SNIMatch) (*v2.Listener, error) {
	filterChains, err := createFilterChainsForTLS(manager, sniMatches)
	if err != nil {
		return nil, err
	}

	return &v2.Listener{
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

func createFilters(manager *httpconnmanagerv2.HttpConnectionManager) ([]*listener.Filter, error) {
	managerAny, err := ptypes.MarshalAny(manager)
	if err != nil {
		return []*listener.Filter{}, err
	}

	return []*listener.Filter{{
		Name:       wellknown.HTTPConnectionManager,
		ConfigType: &listener.Filter_TypedConfig{TypedConfig: managerAny},
	}}, nil
}

func createFilterChainsForTLS(manager *httpconnmanagerv2.HttpConnectionManager, sniMatches []*SNIMatch) ([]*listener.FilterChain, error) {
	res := make([]*listener.FilterChain, 0, len(sniMatches))
	for _, sniMatch := range sniMatches {
		// The connection manager received in the params contains all the
		// matching rules. For each sniMatch, we just need the rules defined for
		// the same hosts defined in the sniMatch
		connManager := filterByDomains(manager, sniMatch.hosts)
		filters, err := createFilters(&connManager)
		if err != nil {
			return nil, err
		}

		tlsContext := createTLSContext(sniMatch.certificateChain, sniMatch.privateKey)
		tlsAny, err := ptypes.MarshalAny(tlsContext)
		if err != nil {
			return nil, err
		}

		filterChain := listener.FilterChain{
			FilterChainMatch: &listener.FilterChainMatch{
				ServerNames: sniMatch.hosts,
			},
			TransportSocket: &core.TransportSocket{
				Name:       "tls",
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
