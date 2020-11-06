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
	"net/http"
	"time"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
)

// NewRoute creates a new Route.
func NewRoute(name string,
	headersMatch []*route.HeaderMatcher,
	path string,
	wrs []*route.WeightedCluster_ClusterWeight,
	routeTimeout time.Duration,
	headers map[string]string) *route.Route {

	return &route.Route{
		Name: name,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: path,
			},
			Headers: headersMatch,
		},
		Action: &route.Route_Route{
			Route: &route.RouteAction{
				ClusterSpecifier: &route.RouteAction_WeightedClusters{
					WeightedClusters: &route.WeightedCluster{
						Clusters: wrs,
					},
				},
				Timeout: ptypes.DurationProto(routeTimeout),
				UpgradeConfigs: []*route.RouteAction_UpgradeConfig{{
					UpgradeType: "websocket",
					Enabled:     &wrappers.BoolValue{Value: true},
				}},
			},
		},
		RequestHeadersToAdd: headersToAdd(headers),
	}
}

// NewRouteStatusOK creates a route that simply returns 200.
func NewRouteStatusOK(name string, path string) *route.Route {
	return &route.Route{
		Name: name,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Path{
				Path: path,
			},
		},
		Action: &route.Route_DirectResponse{
			DirectResponse: &route.DirectResponseAction{Status: http.StatusOK},
		},
	}
}
