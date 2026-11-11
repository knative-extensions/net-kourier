/*
Copyright 2026 The Knative Authors

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

package generator

import (
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"

	envoy "knative.dev/net-kourier/pkg/envoy/api"
)

// statsListener exposes only the allowlisted admin operations. It deliberately
// does not use the application connection manager: metrics must not inherit
// external authorization, tracing, access logging, or PROXY protocol settings.
func statsListener(addresses []string) (*listener.Listener, error) {
	manager := &hcm.HttpConnectionManager{
		StatPrefix: "stats_server",
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
			ConfigType: &hcm.HttpFilter_TypedConfig{TypedConfig: &anypb.Any{
				TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
			}},
		}},
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: &route.RouteConfiguration{
				Name: "stats_routes",
				VirtualHosts: []*route.VirtualHost{{
					Name:    "admin_interface",
					Domains: []string{"*"},
					Routes: []*route.Route{
						statsRoute("GET", "/(certs|stats(/prometheus)?|server_info|clusters|listeners|ready)?"),
						statsRoute("POST", "/drain_listeners"),
					},
				}},
			},
		},
	}
	l, err := envoy.NewHTTPListener(manager, 9000, false, addresses)
	if err != nil {
		return nil, err
	}
	l.Name = "stats_listener"
	return l, nil
}

func statsRoute(method, pathRegex string) *route.Route {
	return &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_SafeRegex{
				SafeRegex: &matcher.RegexMatcher{Regex: pathRegex},
			},
			Headers: []*route.HeaderMatcher{{
				Name: ":method",
				HeaderMatchSpecifier: &route.HeaderMatcher_StringMatch{
					StringMatch: &matcher.StringMatcher{
						MatchPattern: &matcher.StringMatcher_Exact{Exact: method},
					},
				},
			}},
		},
		Action: &route.Route_Route{Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_Cluster{Cluster: "service_stats"},
		}},
	}
}
