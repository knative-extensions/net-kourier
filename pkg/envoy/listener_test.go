package envoy

import (
	"testing"

	"github.com/golang/protobuf/ptypes"

	auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"

	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

func TestCreateHTTPListener(t *testing.T) {
	manager := NewHttpConnectionManager([]*route.VirtualHost{})

	l, err := NewHTTPListener(&manager, 8080)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8080), l.Address.GetSocketAddress().GetPortValue())
	assert.Assert(t, is.Nil(l.FilterChains[0].TransportSocket)) //TLS not configured
}

func TestCreateHTTPSListener(t *testing.T) {
	manager := NewHttpConnectionManager([]*route.VirtualHost{})

	l, err := NewHTTPSListener(&manager, 8081, "some_certificate_chain", "some_private_key")
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, core.SocketAddress_TCP, l.Address.GetSocketAddress().Protocol)
	assert.Equal(t, "0.0.0.0", l.Address.GetSocketAddress().Address)
	assert.Equal(t, uint32(8081), l.Address.GetSocketAddress().GetPortValue())

	// Check that TLS is configured

	downstream := &auth.DownstreamTlsContext{}

	err = ptypes.UnmarshalAny(l.FilterChains[0].TransportSocket.GetTypedConfig(), downstream)
	if err != nil {
		t.Fatal(err)
	}

	certs := downstream.CommonTlsContext.TlsCertificates[0]
	assert.Equal(t, "some_certificate_chain", string(certs.CertificateChain.GetInlineBytes()))
	assert.Equal(t, "some_private_key", string(certs.PrivateKey.GetInlineBytes()))

}
