package envoy

import (
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
)

func NewVirtualHost(name string, domains []string, routes []*route.Route) route.VirtualHost {
	return route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
	}
}

func NewVirtualHostWithExtAuthz(name string, contextExtensions map[string]string, domains []string,
	routes []*route.Route) route.VirtualHost {

	perFilterConfig := extAuthService.ExtAuthzPerRoute{
		Override: &extAuthService.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &extAuthService.CheckSettings{
				ContextExtensions: contextExtensions,
			},
		},
	}

	b := proto.NewBuffer(nil)
	b.SetDeterministic(true)
	_ = b.Marshal(&perFilterConfig)
	filter := &any.Any{
		TypeUrl: "type.googleapis.com/" + proto.MessageName(&perFilterConfig),
		Value:   b.Bytes(),
	}

	r := route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
		TypedPerFilterConfig: map[string]*any.Any{
			wellknown.HTTPExternalAuthorization: filter,
		},
	}

	return r

}
