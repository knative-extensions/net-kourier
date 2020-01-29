package envoy

import (
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
)

func NewLBEndpoint(ip string, port uint32) *endpoint.LbEndpoint {
	serviceEndpoint := &core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.SocketAddress_TCP,
				Address:  ip,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: port,
				},
				Ipv4Compat: true,
			},
		},
	}

	return &endpoint.LbEndpoint{
		HostIdentifier: &endpoint.LbEndpoint_Endpoint{
			Endpoint: &endpoint.Endpoint{
				Address: serviceEndpoint,
			},
		},
	}
}
