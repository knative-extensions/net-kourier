package envoy

import (
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoy_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	"github.com/golang/protobuf/ptypes"

	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
)

func NewHttpConnectionManager(virtualHosts []*route.VirtualHost) httpconnectionmanagerv2.HttpConnectionManager {
	return httpconnectionmanagerv2.HttpConnectionManager{
		CodecType:  httpconnectionmanagerv2.HttpConnectionManager_AUTO,
		StatPrefix: "ingress_http",
		RouteSpecifier: &httpconnectionmanagerv2.HttpConnectionManager_RouteConfig{
			RouteConfig: &v2.RouteConfiguration{
				Name:         "local_route",
				VirtualHosts: virtualHosts,
			},
		},
		HttpFilters: []*httpconnectionmanagerv2.HttpFilter{
			{
				Name: wellknown.Router,
			},
		},
		AccessLog: accessLogs(),
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
