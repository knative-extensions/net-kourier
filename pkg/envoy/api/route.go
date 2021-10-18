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
	"knative.dev/net-kourier/pkg/config"
	"time"

	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NewRoute creates a new Route.
func NewRoute(name string,
	headersMatch []*route.HeaderMatcher,
	path string,
	wrs []*route.WeightedCluster_ClusterWeight,
	routeTimeout time.Duration,
	headers map[string]string,
	hostRewrite string) *route.Route {

	routeAction := &route.RouteAction{
		ClusterSpecifier: &route.RouteAction_WeightedClusters{
			WeightedClusters: &route.WeightedCluster{
				Clusters: wrs,
			},
		},
		Timeout: durationpb.New(routeTimeout),
		UpgradeConfigs: []*route.RouteAction_UpgradeConfig{{
			UpgradeType: "websocket",
			Enabled:     wrapperspb.Bool(true),
		}},
	}

	if hostRewrite != "" {
		routeAction.HostRewriteSpecifier = &route.RouteAction_HostRewriteLiteral{
			HostRewriteLiteral: hostRewrite,
		}
	}

	newRoute := &route.Route{
		Name: name,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: path,
			},
			Headers: headersMatch,
		},
		Action: &route.Route_Route{
			Route: routeAction,
		},
		RequestHeadersToAdd: headersToAdd(headers),
	}

	// add local rate limit spec if it's activated
	if config.LocalRateLimit.Enabled && path != "/ready" {
		newRoute.TypedPerFilterConfig = config.LocalRateLimit.FilterConfig
	}

	return newRoute
}

func NewRedirectRoute(name string,
	headersMatch []*route.HeaderMatcher,
	path string,
) *route.Route {
	return &route.Route{
		Name: name,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: path,
			},
			Headers: headersMatch,
		},
		Action: &route.Route_Redirect{
			Redirect: &route.RedirectAction{
				SchemeRewriteSpecifier: &route.RedirectAction_HttpsRedirect{
					HttpsRedirect: true,
				},
			},
		},
	}
}
