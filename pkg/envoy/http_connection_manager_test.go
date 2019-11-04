package envoy

import (
	"testing"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	pstruct "github.com/golang/protobuf/ptypes/struct"
	"gotest.tools/assert"
)

var testVirtualHosts = []*route.VirtualHost{
	{
		Name:    "helloworld-go",
		Domains: []string{"helloworld-go.default.127.0.0.1.nip.io"},
		Routes: []*route.Route{
			{
				Name: "helloworld-route",
			},
		},
	},
}

func TestCreatesManagerWithVirtualHosts(t *testing.T) {
	connManager := newHttpConnectionManager(testVirtualHosts)

	VirtualHosts := connManager.RouteSpecifier.(*httpconnectionmanagerv2.HttpConnectionManager_RouteConfig).
		RouteConfig.VirtualHosts

	assert.DeepEqual(t, VirtualHosts, testVirtualHosts)
}

func TestCreatesManagerThatOutputsToStdOut(t *testing.T) {
	connManager := newHttpConnectionManager(testVirtualHosts)

	accessLog := connManager.AccessLog[0]
	accessLogPath := accessLog.ConfigType.(*v2.AccessLog_Config).
		Config.Fields["path"].Kind.(*pstruct.Value_StringValue).StringValue

	assert.Equal(t, "/dev/stdout", accessLogPath)
}
