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
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto" //nolint // TODO: Move over to non-deprecated proto package
	"github.com/golang/protobuf/ptypes/any"
)

func NewVirtualHost(name string, domains []string, routes []*route.Route) *route.VirtualHost {
	return &route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
	}
}

func NewVirtualHostWithExtAuthz(
	name string,
	contextExtensions map[string]string,
	domains []string,
	routes []*route.Route) *route.VirtualHost {

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
		TypeUrl: "type.googleapis.com/envoy.config.filter.http.ext_authz.v2.ExtAuthzPerRoute",
		Value:   b.Bytes(),
	}

	return &route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
		TypedPerFilterConfig: map[string]*any.Any{
			wellknown.HTTPExternalAuthorization: filter,
		},
	}

}
