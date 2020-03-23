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
	externalAuthzService "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"net/http"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
)

func NewRoute(name string,
	path string,
	wrs []*route.WeightedCluster_ClusterWeight,
	routeTimeout time.Duration,
	retryAttempts uint32,
	perTryTimeout time.Duration,
	headers map[string]string) *route.Route {

	return &route.Route{
		Name: name,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: path,
			},
		},
		Action: &route.Route_Route{Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: &route.WeightedCluster{
					Clusters: wrs,
				},
			},
			ClusterNotFoundResponseCode: route.RouteAction_SERVICE_UNAVAILABLE,
			Timeout:                     ptypes.DurationProto(routeTimeout),
			UpgradeConfigs: []*route.RouteAction_UpgradeConfig{{
				UpgradeType: "websocket",
				Enabled:     &wrappers.BoolValue{Value: true},
			}},
			RetryPolicy: retryPolicy(retryAttempts, perTryTimeout),
		}},
		RequestHeadersToAdd: headersToAdd(headers),
	}
}

// Creates a route that simply returns 200
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

func NewRouteReadiness(name string, path string, headers map[string]string, extAuthzEnabled bool) *route.Route {

	var routeHeaders []*route.HeaderMatcher
	for k, v := range headers {
		rhm := &route.HeaderMatcher{
			Name: k,
			HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
				ExactMatch: v,
			},
		}
		routeHeaders = append(routeHeaders, rhm)
	}

	r := &route.Route{
		Name: name,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Path{
				Path: path,
			},
			Headers: routeHeaders,
		},
		Action: &route.Route_DirectResponse{
			DirectResponse: &route.DirectResponseAction{Status: http.StatusOK},
		},
	}

	if extAuthzEnabled {
		// We need to disable the exthAuthService for this route, or all the readiness
		// probes will fail otherwise. Safe to disable even when it's not enabled.
		perFilterConfig := externalAuthzService.ExtAuthzPerRoute{
			Override: &externalAuthzService.ExtAuthzPerRoute_Disabled{
				Disabled: true,
			},
		}
		b := proto.NewBuffer(nil)
		b.SetDeterministic(true)
		_ = b.Marshal(&perFilterConfig)
		filter := &any.Any{
			TypeUrl: "type.googleapis.com/" + proto.MessageName(&perFilterConfig),
			Value:   b.Bytes(),
		}

		r.TypedPerFilterConfig = map[string]*any.Any{
			wellknown.HTTPExternalAuthorization: filter,
		}
	}
	return r
}

func retryPolicy(retryAttempts uint32, perTryTimeout time.Duration) *route.RetryPolicy {
	if retryAttempts == 0 {
		return nil
	}

	return &route.RetryPolicy{
		RetryOn: "5xx",
		NumRetries: &wrappers.UInt32Value{
			Value: retryAttempts,
		},
		PerTryTimeout: ptypes.DurationProto(perTryTimeout),
	}
}
