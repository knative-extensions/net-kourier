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
	"reflect"
	"testing"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_api_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	prx "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	auth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"knative.dev/net-kourier/pkg/config"
)

const urlPrefix = "type.googleapis.com/"

func TestNewHTTPListener(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
	}
	manager := NewHTTPConnectionManager("test", &kourierConfig)

	l, err := NewHTTPListener(manager, 8080, false)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8080), l.Address.GetSocketAddress().GetPortValue())
	assert.Assert(t, is.Nil(l.FilterChains[0].TransportSocket)) // TLS not configured

	// check proxy protocol is not configured
	assert.Check(t, len(l.ListenerFilters) == 0)
}

func TestNewHTTPListenerWithProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        true,
		IdleTimeout:                0 * time.Second,
	}
	manager := NewHTTPConnectionManager("test", &kourierConfig)

	l, err := NewHTTPListener(manager, 8080, true)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8080), l.Address.GetSocketAddress().GetPortValue())
	assert.Assert(t, is.Nil(l.FilterChains[0].TransportSocket)) // TLS not configured

	// check proxy protocol is configured
	assertListenerHasProxyProtocolConfigured(t, l.ListenerFilters[0])
}

var c = Certificate{
	Certificate: []byte("some_certificate_chain"),
	PrivateKey:  []byte("some_private_key"),
}

var crypto = Certificate{
	Certificate:        []byte("some_certificate_chain"),
	PrivateKey:         []byte("some_private_key"),
	PrivateKeyProvider: "cryptomb",
	PollDelay:          durationpb.New(10 * time.Millisecond),
}

func TestNewHTTPSListener(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
	}
	manager := NewHTTPConnectionManager("test", &kourierConfig)

	filterChain, err := CreateFilterChainFromCertificateAndPrivateKey(manager, &c)
	assert.NilError(t, err)

	l, err := NewHTTPSListener(8081, []*envoy_api_v3.FilterChain{filterChain}, false)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8081), l.Address.GetSocketAddress().GetPortValue())

	// Check that TLS is configured
	gotCertChain, gotPrivateKey, _, err := getTLSCreds(l.FilterChains[0])
	assert.NilError(t, err)

	assert.DeepEqual(t, c.Certificate, gotCertChain)
	assert.DeepEqual(t, c.PrivateKey, gotPrivateKey)

	// check proxy protocol is not configured
	assert.Check(t, len(l.ListenerFilters) == 0)
}

func TestNewHTTPSListenerWithPrivatekeyProvider(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
		EnableCryptoMB:             true,
	}
	manager := NewHTTPConnectionManager("test", &kourierConfig)

	msg, err := c.createCryptoMbMessaage()
	assert.NilError(t, err)

	fakePrivateKeyProvider := &auth.PrivateKeyProvider{
		ProviderName: "cryptomb",
		ConfigType: &auth.PrivateKeyProvider_TypedConfig{
			TypedConfig: msg,
		},
	}

	filterChain, err := CreateFilterChainFromCertificateAndPrivateKey(manager, &crypto)
	assert.NilError(t, err)

	l, err := NewHTTPSListener(8081, []*envoy_api_v3.FilterChain{filterChain}, false)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8081), l.Address.GetSocketAddress().GetPortValue())

	// Check that TLS is configured
	gotCertChain, gotPrivateKey, gotProvider, err := getTLSCreds(l.FilterChains[0])
	assert.NilError(t, err)

	assert.DeepEqual(t, c.Certificate, gotCertChain)
	assert.DeepEqual(t, []byte(nil), gotPrivateKey)
	assert.DeepEqual(t, fakePrivateKeyProvider.String(), gotProvider.String())

	// check proxy protocol is not configured
	assert.Check(t, len(l.ListenerFilters) == 0)
}
func TestNewHTTPSListenerWithProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        true,
		IdleTimeout:                0 * time.Second,
	}
	manager := NewHTTPConnectionManager("test", &kourierConfig)

	filterChain, err := CreateFilterChainFromCertificateAndPrivateKey(manager, &c)
	assert.NilError(t, err)

	l, err := NewHTTPSListener(8081, []*envoy_api_v3.FilterChain{filterChain}, true)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8081), l.Address.GetSocketAddress().GetPortValue())

	// Check that TLS is configured
	gotCertChain, gotPrivateKey, _, err := getTLSCreds(l.FilterChains[0])
	assert.NilError(t, err)

	assert.DeepEqual(t, c.Certificate, gotCertChain)
	assert.DeepEqual(t, c.PrivateKey, gotPrivateKey)
	// check proxy protocol is configured
	assertListenerHasProxyProtocolConfigured(t, l.ListenerFilters[0])
}

func TestNewHTTPSListenerWithSNI(t *testing.T) {
	sniMatches := []*SNIMatch{{
		Hosts:            []string{"some_host.com"},
		CertificateChain: []byte("cert1"),
		PrivateKey:       []byte("key1"),
	}, {
		Hosts:            []string{"another_host.com"},
		CertificateChain: []byte("cert2"),
		PrivateKey:       []byte("key2"),
	}}

	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
	}
	manager := NewHTTPConnectionManager("test", &kourierConfig)
	listener, err := NewHTTPSListenerWithSNI(manager, 8443, sniMatches, false)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, listener.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", listener.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8443), listener.Address.GetSocketAddress().GetPortValue())

	// Listener Filter required for SNI
	// we should only have one listener filter when proxy protocol is disabled
	assert.Check(t, listener.ListenerFilters[0].Name != wellknown.ProxyProtocol)
	assert.Equal(t, listener.ListenerFilters[0].Name, wellknown.TlsInspector)

	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[0])
	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[1])
}

func TestNewHTTPSListenerWithSNIWithProxyProtocol(t *testing.T) {
	sniMatches := []*SNIMatch{{
		Hosts:            []string{"some_host.com"},
		CertificateChain: []byte("cert1"),
		PrivateKey:       []byte("key1"),
	}, {
		Hosts:            []string{"another_host.com"},
		CertificateChain: []byte("cert2"),
		PrivateKey:       []byte("key2"),
	}}
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        true,
		IdleTimeout:                0 * time.Second,
	}
	manager := NewHTTPConnectionManager("test", &kourierConfig)
	listener, err := NewHTTPSListenerWithSNI(manager, 8443, sniMatches, true)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, listener.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", listener.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8443), listener.Address.GetSocketAddress().GetPortValue())

	// check both tls inspector and proxy protocol are configured
	assertListenerHasProxyProtocolConfigured(t, listener.ListenerFilters[0])
	assert.Equal(t, listener.ListenerFilters[1].Name, wellknown.TlsInspector)

	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[0])
	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[1])
}

func assertListenerHasSNIMatchConfigured(t *testing.T, listener *envoy_api_v3.Listener, match *SNIMatch) {
	filterChainFirstSNIMatch := getFilterChainByServerName(listener, match.Hosts)
	assert.Assert(t, filterChainFirstSNIMatch != nil)

	certChain, privateKey, _, err := getTLSCreds(filterChainFirstSNIMatch)
	assert.NilError(t, err)
	assert.DeepEqual(t, match.CertificateChain, certChain)
	assert.DeepEqual(t, match.PrivateKey, privateKey)
}

func assertListenerHasProxyProtocolConfigured(t *testing.T, listenerFilter *envoy_api_v3.ListenerFilter) {
	typeURL := urlPrefix + string((&prx.ProxyProtocol{}).ProtoReflect().Descriptor().FullName())
	assert.Equal(t, wellknown.ProxyProtocol, listenerFilter.GetName())
	assert.Equal(t, typeURL, listenerFilter.GetTypedConfig().GetTypeUrl())
}

func getFilterChainByServerName(listener *envoy_api_v3.Listener, serverNames []string) *envoy_api_v3.FilterChain {
	for _, filterChain := range listener.FilterChains {
		filterChainMatch := filterChain.GetFilterChainMatch()

		if filterChainMatch != nil && reflect.DeepEqual(filterChainMatch.ServerNames, serverNames) {
			return filterChain
		}
	}

	return nil
}

// Note: Returns an error when there are multiple certificates
func getTLSCreds(filterChain *envoy_api_v3.FilterChain) (certChain []byte, privateKey []byte, privateKeyProvider *auth.PrivateKeyProvider, err error) {
	downstreamTLSContext := &auth.DownstreamTlsContext{}
	err = anypb.UnmarshalTo(filterChain.GetTransportSocket().GetTypedConfig(), downstreamTLSContext, proto.UnmarshalOptions{})
	if err != nil {
		return nil, nil, nil, err
	}

	if len(downstreamTLSContext.CommonTlsContext.TlsCertificates) > 1 {
		return nil, nil, nil, fmt.Errorf("more than one certificate configured")
	}

	certs := downstreamTLSContext.CommonTlsContext.TlsCertificates[0]
	certChain = certs.CertificateChain.GetInlineBytes()
	privateKey = certs.PrivateKey.GetInlineBytes()
	privateKeyProvider = certs.PrivateKeyProvider

	return certChain, privateKey, privateKeyProvider, nil
}
