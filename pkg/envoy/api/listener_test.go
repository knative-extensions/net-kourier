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

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

func TestNewHTTPListener(t *testing.T) {
	manager := NewHTTPConnectionManager()

	l, err := NewHTTPListener(manager, 8080)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8080), l.Address.GetSocketAddress().GetPortValue())
	assert.Assert(t, is.Nil(l.FilterChains[0].TransportSocket)) //TLS not configured
}

func TestNewHTTPSListener(t *testing.T) {
	manager := NewHTTPConnectionManager()

	certChain := []byte("some_certificate_chain")
	privateKey := []byte("some_private_key")

	l, err := NewHTTPSListener(manager, 8081, certChain, privateKey)
	if err != nil {
		t.Error(err)
	}

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
		hosts:            []string{"some_host.com"},
		certificateChain: []byte("cert1"),
		privateKey:       []byte("key1"),
	}, {
		hosts:            []string{"another_host.com"},
		certificateChain: []byte("cert2"),
		privateKey:       []byte("key2"),
	}}

	manager := NewHTTPConnectionManager()
	listener, err := NewHTTPSListenerWithSNI(manager, 8443, sniMatches)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, core.SocketAddress_TCP, listener.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", listener.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8443), listener.Address.GetSocketAddress().GetPortValue())

	// Listener Filter required for SNI
	assert.Equal(t, listener.ListenerFilters[0].Name, wellknown.TlsInspector)

	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[0])
	assertListenerHasSNIMatchConfigured(t, listener, sniMatches[1])
}

func assertListenerHasSNIMatchConfigured(t *testing.T, listener *envoy_api_v2.Listener, match *SNIMatch) {
	filterChainFirstSNIMatch := getFilterChainByServerName(listener, match.hosts)
	assert.Assert(t, filterChainFirstSNIMatch != nil)

	certChain, privateKey, err := getTLSCreds(filterChainFirstSNIMatch)
	assert.NilError(t, err)
	assert.DeepEqual(t, match.certificateChain, certChain)
	assert.DeepEqual(t, match.privateKey, privateKey)
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
