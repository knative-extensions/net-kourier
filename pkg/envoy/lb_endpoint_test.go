package envoy

import (
	"testing"

	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"gotest.tools/assert"

	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
)

func TestNewLBEndpoint(t *testing.T) {
	ip := "127.0.0.1"
	port := uint32(8080)

	endpoint := NewLBEndpoint(ip, port)

	lbEndpoint := endpoint.GetHostIdentifier().(*envoy_api_v2_endpoint.LbEndpoint_Endpoint).Endpoint
	socketAddress := lbEndpoint.GetAddress().GetSocketAddress()
	assert.Equal(t, ip, socketAddress.Address)
	assert.Equal(t, port, socketAddress.PortSpecifier.(*core.SocketAddress_PortValue).PortValue)
}
