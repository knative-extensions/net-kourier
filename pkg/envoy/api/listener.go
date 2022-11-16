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
	"errors"
	"fmt"
	"time"

	cryptomb "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/private_key_providers/cryptomb/v3alpha"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	prx "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	auth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
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
func NewHTTPListener(manager *hcm.HttpConnectionManager, port uint32, enableProxyProtocol bool) (*listener.Listener, error) {
	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	var listenerFilter []*listener.ListenerFilter
	if enableProxyProtocol {
		proxyProtocolListenerFilter, err := createProxyProtocolListenerFilter()
		if err != nil {
			return nil, err
		}
		listenerFilter = append(listenerFilter, proxyProtocolListenerFilter)
	}

	return &listener.Listener{
		Name:            CreateListenerName(port),
		Address:         createAddress(port),
		ListenerFilters: listenerFilter,
		FilterChains: []*listener.FilterChain{{
			Filters: filters,
		}},
	}, nil
}

// NewHTTPSListener creates a new Listener at the given port with a given filter chain
func NewHTTPSListener(port uint32, filterChain []*listener.FilterChain, enableProxyProtocol bool) (*listener.Listener, error) {
	var listenerFilter []*listener.ListenerFilter
	if enableProxyProtocol {
		proxyProtocolListenerFilter, err := createProxyProtocolListenerFilter()
		if err != nil {
			return nil, err
		}
		listenerFilter = append(listenerFilter, proxyProtocolListenerFilter)
	}

	return &listener.Listener{
		Name:            CreateListenerName(port),
		Address:         createAddress(port),
		ListenerFilters: listenerFilter,
		FilterChains:    filterChain,
	}, nil
}

// CreateFilterChainFromCertificateAndPrivateKey creates a new filter chain from a certificate and a private key
func CreateFilterChainFromCertificateAndPrivateKey(
	manager *hcm.HttpConnectionManager,
	cert *Certificate) (*listener.FilterChain, error) {

	filters, err := createFilters(manager)
	if err != nil {
		return nil, err
	}

	tlsContext, err := cert.createTLSContext()
	if err != nil {
		return nil, err
	}
	tlsAny, err := anypb.New(tlsContext)
	if err != nil {
		return nil, err
	}

	return &listener.FilterChain{
		Filters: filters,
		TransportSocket: &core.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: tlsAny},
		},
	}, nil
}

// NewHTTPSListenerWithSNI creates a new Listener at the given port, backed by the given
// manager and applies a FilterChain with the given sniMatches.
//
// Ref: https://www.envoyproxy.io/docs/envoy/latest/faq/configuration/sni.html
func NewHTTPSListenerWithSNI(manager *hcm.HttpConnectionManager, port uint32, sniMatches []*SNIMatch, enableProxyProtocol bool) (*listener.Listener, error) {
	filterChains, err := createFilterChainsForTLS(manager, sniMatches)
	if err != nil {
		return nil, err
	}

	var listenerFilter []*listener.ListenerFilter

	// proxy protocol listener filter should be executed before the TLS inspector listener filter.
	// Since the proxy protocol adds bytes to the beginning of the connection,
	// the SNI will not be parsed correctly if the proxy protocol listener filter is not executed first.
	// Without SNI matching, you would get the wrong certificate, and traffic would drop.
	// https://github.com/solo-io/gloo/issues/5116
	if enableProxyProtocol {
		proxyProtocolListenerFilter, err := createProxyProtocolListenerFilter()
		if err != nil {
			return nil, err
		}
		listenerFilter = append(listenerFilter, proxyProtocolListenerFilter)
	}

	listenerFilterForTLS := &listener.ListenerFilter{
		// TLS Inspector listener filter must be configured in order to
		// detect requested SNI.
		// Ref: https://www.envoyproxy.io/docs/envoy/latest/faq/configuration/sni.html
		Name: wellknown.TlsInspector,
		ConfigType: &listener.ListenerFilter_TypedConfig{TypedConfig: &anypb.Any{
			TypeUrl: "type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector",
		}},
	}

	listenerFilter = append(listenerFilter, listenerFilterForTLS)

	return &listener.Listener{
		Name:            CreateListenerName(port),
		Address:         createAddress(port),
		FilterChains:    filterChains,
		ListenerFilters: listenerFilter,
	}, nil
}

// CreateListenerName returns a listener name based on port
func CreateListenerName(port uint32) string {
	return fmt.Sprintf("listener_%d", port)
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

		c := Certificate{Certificate: sniMatch.CertificateChain, PrivateKey: sniMatch.PrivateKey}

		tlsContext, err := c.createTLSContext()
		if err != nil {
			return nil, err
		}
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

// Certificate stores certificate data to generrate TLS context for downstream.
type Certificate struct {
	Certificate        []byte
	PrivateKey         []byte
	PrivateKeyProvider string
	PollDelay          *durationpb.Duration
}

// messageToAny converts from proto message to proto Any
func messageToAny(msg proto.Message) (*anypb.Any, error) {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		err = fmt.Errorf("error marshaling message %s: %w", prototext.Format(msg), err)
		return nil, err
	}
	return &anypb.Any{
		// nolint: staticcheck
		TypeUrl: "type.googleapis.com/" + string(msg.ProtoReflect().Descriptor().FullName()),
		Value:   b,
	}, nil
}

func (c Certificate) createCryptoMbMessaage() (*anypb.Any, error) {
	config := cryptomb.CryptoMbPrivateKeyMethodConfig{
		// Hardcoded to 10ms, it will be configurable in the future.
		PollDelay: durationpb.New(10 * time.Millisecond),
		PrivateKey: &core.DataSource{
			Specifier: &core.DataSource_InlineBytes{
				InlineBytes: c.PrivateKey,
			},
		},
	}
	return messageToAny(&config)
}

func (c Certificate) createTLSContext() (*auth.DownstreamTlsContext, error) {
	tlsCertificates, err := c.createTLScertificates()
	if err != nil {
		return nil, err
	}

	return &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			AlpnProtocols: []string{"h2", "http/1.1"},
			// Temporary fix until we start using envoyproxy image newer than v1.23.0 (envoyproxy has adopted TLS v1.2 as the default minimum version in https://github.com/envoyproxy/envoy/commit/f8baa480ec9c6cbaa7a9d5433102efb04145cfc8)
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: auth.TlsParameters_TLSv1_2,
			},
			TlsCertificates: []*auth.TlsCertificate{tlsCertificates},
		},
	}, nil
}

func (c Certificate) createTLScertificates() (*auth.TlsCertificate, error) {
	switch c.PrivateKeyProvider {
	case "":
		return &auth.TlsCertificate{
			CertificateChain: &core.DataSource{
				Specifier: &core.DataSource_InlineBytes{InlineBytes: c.Certificate},
			},
			PrivateKey: &core.DataSource{
				Specifier: &core.DataSource_InlineBytes{InlineBytes: c.PrivateKey},
			}}, nil
	case "cryptomb":
		msg, err := c.createCryptoMbMessaage()
		if err != nil {
			return nil, err
		}
		return &auth.TlsCertificate{
			CertificateChain: &core.DataSource{
				Specifier: &core.DataSource_InlineBytes{InlineBytes: c.Certificate},
			},
			PrivateKeyProvider: &auth.PrivateKeyProvider{
				ProviderName: "cryptomb",
				ConfigType: &auth.PrivateKeyProvider_TypedConfig{
					TypedConfig: msg,
				},
			}}, nil
	default:
		return nil, errors.New("Unsupported private key provider: " + c.PrivateKeyProvider)
	}
}

// Ref: https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listener_filters/proxy_protocol
func createProxyProtocolListenerFilter() (listenerFilter *listener.ListenerFilter, err error) {
	listenerFilterConfig, err := anypb.New(&prx.ProxyProtocol{})
	if err != nil {
		return nil, err
	}

	return &listener.ListenerFilter{
		Name: wellknown.ProxyProtocol,
		ConfigType: &listener.ListenerFilter_TypedConfig{
			TypedConfig: listenerFilterConfig,
		},
	}, nil
}
