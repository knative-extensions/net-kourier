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
	envoy_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	fileaccesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	"knative.dev/net-kourier/pkg/config"
)

// NewHTTPConnectionManager creates a new HttpConnectionManager that points to the given
// RouteConfig for further configuration.
func NewHTTPConnectionManager(routeConfigName string) *httpconnectionmanagerv2.HttpConnectionManager {
	filters := make([]*httpconnectionmanagerv2.HttpFilter, 0, 1)

	if config.ExternalAuthz.Enabled {
		filters = append(filters, config.ExternalAuthz.HTTPFilter)
	}

	// Append the Router filter at the end.
	filters = append(filters, &httpconnectionmanagerv2.HttpFilter{
		Name: wellknown.Router,
	})
	// Write access logs to stdout by default.
	accessLog, _ := ptypes.MarshalAny(&fileaccesslog.FileAccessLog{
		Path:            "/dev/stdout",
		AccessLogFormat: nil,
	})

	return &httpconnectionmanagerv2.HttpConnectionManager{
		CodecType:   httpconnectionmanagerv2.HttpConnectionManager_AUTO,
		StatPrefix:  "ingress_http",
		HttpFilters: filters,
		AccessLog: []*envoy_accesslog_v2.AccessLog{{
			Name: wellknown.FileAccessLog,
			ConfigType: &envoy_accesslog_v2.AccessLog_TypedConfig{
				TypedConfig: accessLog,
			},
		}},
		RouteSpecifier: &httpconnectionmanagerv2.HttpConnectionManager_Rds{
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
		},
	}
}

// NewRouteConfig create a new RouteConfiguration with the given name and hosts.
func NewRouteConfig(name string, virtualHosts []*route.VirtualHost) *route.RouteConfiguration {
	return &route.RouteConfiguration{
		Name:         name,
		VirtualHosts: virtualHosts,
	}
}
