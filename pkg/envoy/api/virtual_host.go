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
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"
)

// NewVirtualHost creates a new VirtualHost.
func NewVirtualHost(name string, domains []string, routes []*route.Route) *route.VirtualHost {
	return &route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
	}
}

// NewVirtualHostWithExtAuthz creates a new VirtualHost with ExtAuthz settings.
func NewVirtualHostWithExtAuthz(
	name string,
	contextExtensions map[string]string,
	domains []string,
	routes []*route.Route) *route.VirtualHost {

	filter, _ := anypb.New(&extAuthService.ExtAuthzPerRoute{
		Override: &extAuthService.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &extAuthService.CheckSettings{
				ContextExtensions: contextExtensions,
			},
		},
	})

	return &route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
		TypedPerFilterConfig: map[string]*anypb.Any{
			wellknown.HTTPExternalAuthorization: filter,
		},
	}

}
