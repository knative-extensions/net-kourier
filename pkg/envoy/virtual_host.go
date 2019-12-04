package envoy

import (
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
)

func NewVirtualHost(name string, domains []string, routes []*route.Route) route.VirtualHost {
	return route.VirtualHost{Name: name, Domains: domains, Routes: routes}
}
