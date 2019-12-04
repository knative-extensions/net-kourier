package envoy

import (
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
	headers map[string]string) route.Route {

	return route.Route{
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
			Timeout: ptypes.DurationProto(routeTimeout),
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
func NewRouteStatusOK(name string, path string) route.Route {
	return route.Route{
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

func retryPolicy(retryAttempts uint32, perTryTimeout time.Duration) *route.RetryPolicy {
	if retryAttempts <= 0 {
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
