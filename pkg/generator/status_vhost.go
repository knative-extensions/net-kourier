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

package generator

import (
	"time"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
)

// Generates an internal virtual host that signals that the Envoy instance has
// been configured, this endpoint is used by the kubernetes readiness probe.
func statusVHost() *route.VirtualHost {
	vhost := envoy.NewVirtualHost(
		config.InternalKourierDomain,
		[]string{config.InternalKourierDomain},
		[]*route.Route{readyRoute()},
	)

	// Make sure that ExtAuthz configuration is ignored on this path.
	filter, _ := ptypes.MarshalAny(&extAuthService.ExtAuthzPerRoute{
		Override: &extAuthService.ExtAuthzPerRoute_Disabled{
			Disabled: true,
		},
	})

	vhost.TypedPerFilterConfig = map[string]*any.Any{
		wellknown.HTTPExternalAuthorization: filter,
	}

	return vhost
}

func readyRoute() *route.Route {
	cluster := envoy.NewWeightedCluster("service_stats", 100, nil)
	var wrs []*route.WeightedCluster_ClusterWeight
	wrs = append(wrs, cluster)
	route := envoy.NewRoute("gateway_ready", nil, "/ready", wrs, 1*time.Second, nil, "")

	return route
}
