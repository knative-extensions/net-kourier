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

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	auth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestNewHTTPListener(t *testing.T) {
	manager := NewHTTPConnectionManager("test")

	l, err := NewHTTPListener(manager, 8080)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8080), l.Address.GetSocketAddress().GetPortValue())
	assert.Assert(t, is.Nil(l.FilterChains[0].TransportSocket)) // TLS not configured
}

func TestNewHTTPSListener(t *testing.T) {
	manager := NewHTTPConnectionManager("test")

	certChain := []byte("some_certificate_chain")
	privateKey := []byte("some_private_key")

	l, err := NewHTTPSListener(manager, 8081, certChain, privateKey)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8081), l.Address.GetSocketAddress().GetPortValue())

	// Check that TLS is configured
	gotCertChain, gotPrivateKey, err := getTLSCreds(l.FilterChains[0])
	assert.NilError(t, err)

	assert.DeepEqual(t, certChain, gotCertChain)
	assert.DeepEqual(t, privateKey, gotPrivateKey)
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

	manager := NewHTTPConnectionManager("test")
	listener, err := NewHTTPSListenerWithSNI(manager, 8443, sniMatches)
	assert.NilError(t, err)

	assert.Equal(t, core.SocketAddress_TCP, listener.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", listener.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8443), listener.Address.GetSocketAddress().GetPortValue())

	// Listener Filter required for SNI
	assert.Equal(t, listener.ListenerFilters[0].Name, wellknown.TlsInspector)

	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[0])
	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[1])
}

func assertListenerHasSNIMatchConfigured(t *testing.T, listener *envoy_api_v2.Listener, match *SNIMatch) {
	filterChainFirstSNIMatch := getFilterChainByServerName(listener, match.Hosts)
	assert.Assert(t, filterChainFirstSNIMatch != nil)

	certChain, privateKey, err := getTLSCreds(filterChainFirstSNIMatch)
	assert.NilError(t, err)
	assert.DeepEqual(t, match.CertificateChain, certChain)
	assert.DeepEqual(t, match.PrivateKey, privateKey)
}

func getFilterChainByServerName(listener *envoy_api_v2.Listener, serverNames []string) *envoy_api_v2_listener.FilterChain {
	for _, filterChain := range listener.FilterChains {
		filterChainMatch := filterChain.GetFilterChainMatch()

		if filterChainMatch != nil && reflect.DeepEqual(filterChainMatch.ServerNames, serverNames) {
			return filterChain
		}
	}

	return nil
}

// Note: Returns an error when there are multiple certificates
func getTLSCreds(filterChain *envoy_api_v2_listener.FilterChain) (certChain []byte, privateKey []byte, err error) {
	downstreamTLSContext := &auth.DownstreamTlsContext{}
	err = ptypes.UnmarshalAny(
		filterChain.GetTransportSocket().GetTypedConfig(), downstreamTLSContext,
	)
	if err != nil {
		return nil, nil, err
	}

	if len(downstreamTLSContext.CommonTlsContext.TlsCertificates) > 1 {
		return nil, nil, fmt.Errorf("more than one certificate configured")
	}

	certs := downstreamTLSContext.CommonTlsContext.TlsCertificates[0]
	certChain = certs.CertificateChain.GetInlineBytes()
	privateKey = certs.PrivateKey.GetInlineBytes()

	return certChain, privateKey, nil
}
