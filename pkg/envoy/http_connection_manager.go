package envoy

import (
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	accesslogv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	pstruct "github.com/golang/protobuf/ptypes/struct"
)

func newHttpConnectionManager(virtualHosts []*route.VirtualHost) httpconnectionmanagerv2.HttpConnectionManager {
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
func accessLogs() []*accesslogv2.AccessLog {
	accessLogConfigFields := make(map[string]*pstruct.Value)
	accessLogConfigFields["path"] = &pstruct.Value{
		Kind: &pstruct.Value_StringValue{
			StringValue: "/dev/stdout",
		},
	}

	return []*accesslogv2.AccessLog{
		{
			Name: "envoy.file_access_log",
			ConfigType: &accesslogv2.AccessLog_Config{
				Config: &pstruct.Struct{Fields: accessLogConfigFields},
			},
		},
	}
}
