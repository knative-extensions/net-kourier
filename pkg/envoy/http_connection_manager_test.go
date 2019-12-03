package envoy

import (
	"testing"

	accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	"github.com/golang/protobuf/ptypes"

	envoy_config_filter_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
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
	connManager := NewHttpConnectionManager(testVirtualHosts)

	VirtualHosts := connManager.RouteSpecifier.(*httpconnectionmanagerv2.HttpConnectionManager_RouteConfig).
		RouteConfig.VirtualHosts

	assert.DeepEqual(t, VirtualHosts, testVirtualHosts)
}

func TestCreatesManagerThatOutputsToStdOut(t *testing.T) {
	connManager := NewHttpConnectionManager(testVirtualHosts)
	accessLog := connManager.AccessLog[0]
	accessLogPathAny := accessLog.ConfigType.(*envoy_config_filter_accesslog_v2.AccessLog_TypedConfig).TypedConfig
	fileAccesLog := &accesslog_v2.FileAccessLog{}

	err := ptypes.UnmarshalAny(accessLogPathAny, fileAccesLog)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, "/dev/stdout", fileAccesLog.Path)
}
