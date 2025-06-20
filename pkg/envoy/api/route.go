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
	"time"

	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/any"
	"google.golang.org/protobuf/types/known/anypb"
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
	hostRewrite string,
) *route.Route {
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

	return &route.Route{
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

func NewRouteExtAuthzDisabled(name string,
	headersMatch []*route.HeaderMatcher,
	path string,
	wrs []*route.WeightedCluster_ClusterWeight,
	routeTimeout time.Duration,
	headers map[string]string,
	hostRewrite string,
) *route.Route {
	newRoute := NewRoute(name, headersMatch, path, wrs, routeTimeout, headers, hostRewrite)
	extAuthzDisabled, _ := anypb.New(&extAuthService.ExtAuthzPerRoute{
		Override: &extAuthService.ExtAuthzPerRoute_Disabled{
			Disabled: true,
		},
	})
	newRoute.TypedPerFilterConfig = map[string]*any.Any{
		wellknown.HTTPExternalAuthorization: extAuthzDisabled,
	}

	return newRoute
}
