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
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoy_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	"knative.dev/net-kourier/pkg/config"
)

func NewHTTPConnectionManager(virtualHosts []*route.VirtualHost) *httpconnectionmanagerv2.HttpConnectionManager {

	var filters []*httpconnectionmanagerv2.HttpFilter

	if config.ExternalAuthz.Enabled {
		filters = append(filters, config.ExternalAuthz.HTTPFilter)
	}

	// Append the Router filter at the end.
	routerFilter := httpconnectionmanagerv2.HttpFilter{
		Name: wellknown.Router,
	}
	filters = append(filters, &routerFilter)

	return &httpconnectionmanagerv2.HttpConnectionManager{
		CodecType:  httpconnectionmanagerv2.HttpConnectionManager_AUTO,
		StatPrefix: "ingress_http",
		RouteSpecifier: &httpconnectionmanagerv2.HttpConnectionManager_RouteConfig{
			RouteConfig: &v2.RouteConfiguration{
				Name:         "local_route",
				VirtualHosts: virtualHosts,
			},
		},
		HttpFilters: filters,
		AccessLog:   accessLogs(),
	}
}

func NewRDSHTTPConnectionManager(routeConfigName string) *httpconnectionmanagerv2.HttpConnectionManager_Rds {
	return &httpconnectionmanagerv2.HttpConnectionManager_Rds{
		Rds: &httpconnectionmanagerv2.Rds{
			ConfigSource: &envoy_api_v2_core.ConfigSource{
				ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_Ads{
					Ads: &envoy_api_v2_core.AggregatedConfigSource{},
				},
				InitialFetchTimeout: &duration.Duration{
					Seconds: 10,
					Nanos:   0,
				},
			},
			RouteConfigName: routeConfigName,
		},
	}
}

// Outputs to /dev/stdout using the default format
func accessLogs() []*envoy_accesslog_v2.AccessLog {
	accessLog, _ := ptypes.MarshalAny(&accesslog_v2.FileAccessLog{
		Path: "/dev/stdout",
	})

	return []*envoy_accesslog_v2.AccessLog{
		{
			Name: "envoy.file_access_log",
			ConfigType: &envoy_accesslog_v2.AccessLog_TypedConfig{
				TypedConfig: accessLog,
			},
		},
	}
}

// Returns a copy of the HttpConnectionManager received in the params changing
// only the virtual hosts in the route config. It filters the virtual hosts
// keeping only the ones whose domains contain all the domains received in the
// params.
func filterByDomains(connManager *httpconnectionmanagerv2.HttpConnectionManager, domains []string) *httpconnectionmanagerv2.HttpConnectionManager {
	currentVirtualHosts := connManager.GetRouteSpecifier().(*httpconnectionmanagerv2.HttpConnectionManager_RouteConfig).RouteConfig.GetVirtualHosts()

	res := NewHTTPConnectionManager(currentVirtualHosts)

	var newVirtualHosts []*route.VirtualHost
	for _, virtualHost := range currentVirtualHosts {
		if containsAll(virtualHost.Domains, domains) {
			newVirtualHosts = append(newVirtualHosts, virtualHost)
		}
	}

	res.GetRouteSpecifier().(*httpconnectionmanagerv2.HttpConnectionManager_RouteConfig).RouteConfig.VirtualHosts = newVirtualHosts

	return res
}

// Returns true if slice contains all the items in elems
func containsAll(slice []string, elems []string) bool {
	for _, elem := range elems {
		contained := false

		for _, s := range slice {
			if elem == s {
				contained = true
				break
			}
		}

		if !contained {
			return false
		}
	}

	return true
}
