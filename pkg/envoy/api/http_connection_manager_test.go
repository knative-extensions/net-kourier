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
	"math"
	"testing"
	"time"

	envoy_config_filter_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	fileaccesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gotest.tools/v3/assert"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
)

func TestNewHTTPConnectionManagerWithoutAccessLogWithoutProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: false,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
	}
	connManager := NewHTTPConnectionManager("test", &kourierConfig)
	assert.Check(t, len(connManager.AccessLog) == 0)
	assert.Check(t, connManager.UseRemoteAddress.Value == false)
}

func TestNewHTTPConnectionManagerWithAccessLogWithoutProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
	}
	connManager := NewHTTPConnectionManager("test", &kourierConfig)
	assert.Check(t, connManager.UseRemoteAddress.Value == false)
	accessLog := connManager.AccessLog[0]
	accessLogPathAny := accessLog.ConfigType.(*envoy_config_filter_accesslog_v3.AccessLog_TypedConfig).TypedConfig
	fileAccesLog := &fileaccesslog.FileAccessLog{}

	err := anypb.UnmarshalTo(accessLogPathAny, fileAccesLog, proto.UnmarshalOptions{})
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, "/dev/stdout", fileAccesLog.Path)
}

func TestNewHTTPConnectionManagerWithoutAccessLogWithProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: false,
		EnableProxyProtocol:        true,
		IdleTimeout:                0 * time.Second,
	}
	connManager := NewHTTPConnectionManager("test", &kourierConfig)
	assert.Check(t, len(connManager.AccessLog) == 0)
	assert.Check(t, connManager.UseRemoteAddress != nil)
	assert.Check(t, connManager.UseRemoteAddress.Value)
}

func TestNewHTTPConnectionManagerWithAccessLogWithProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        true,
		IdleTimeout:                0 * time.Second,
	}
	connManager := NewHTTPConnectionManager("test", &kourierConfig)
	assert.Check(t, connManager.UseRemoteAddress != nil)
	assert.Check(t, connManager.UseRemoteAddress.Value)
	accessLog := connManager.AccessLog[0]
	accessLogPathAny := accessLog.ConfigType.(*envoy_config_filter_accesslog_v3.AccessLog_TypedConfig).TypedConfig
	fileAccesLog := &fileaccesslog.FileAccessLog{}

	err := anypb.UnmarshalTo(accessLogPathAny, fileAccesLog, proto.UnmarshalOptions{})
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, "/dev/stdout", fileAccesLog.Path)
}

func TestNewRouteConfig(t *testing.T) {
	vhost := NewVirtualHost(
		"test",
		[]string{"foo", "bar"},
		[]*route.Route{{Name: "baz"}})

	got := NewRouteConfig("test", []*route.VirtualHost{vhost})
	want := &route.RouteConfiguration{
		Name:             "test",
		VirtualHosts:     []*route.VirtualHost{vhost},
		ValidateClusters: wrapperspb.Bool(true),
	}

	assert.DeepEqual(t, got, want, protocmp.Transform())
}

func TestNewHTTPConnectionManagerWithTrustedHops(t *testing.T) {
	tests := []struct {
		name              string
		configKourer      config.Kourier
		wantedTrustedHops uint32
	}{
		{
			name: "test 0 trusted hops",
			configKourer: config.Kourier{
				TrustedHopsCount: 0,
			},
			wantedTrustedHops: 0,
		},
		{
			name: "test 1 trusted hops",
			configKourer: config.Kourier{
				TrustedHopsCount: 1,
			},
			wantedTrustedHops: 1,
		},
		{
			name: "test max trusted hops",
			configKourer: config.Kourier{
				TrustedHopsCount: math.MaxUint32,
			},
			wantedTrustedHops: math.MaxUint32,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connManager := NewHTTPConnectionManager("test", &test.configKourer)
			assert.Equal(t, test.wantedTrustedHops, connManager.XffNumTrustedHops)
		})
	}
}

func TestNewHTTPConnectionManagerWithUseRemoteAddress(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: false,
		UseRemoteAddress:           true,
		IdleTimeout:                0 * time.Second,
	}
	connManager := NewHTTPConnectionManager("test", &kourierConfig)
	assert.Check(t, connManager.UseRemoteAddress.Value == true)
}

func TestNewHTTPConnectionManagerWithDisableEnvoyServerHeader(t *testing.T) {
	tests := []struct {
		name                             string
		configKourer                     config.Kourier
		wantedServerHeaderTransformation hcm.HttpConnectionManager_ServerHeaderTransformation
	}{
		{
			name: "test disable envoy server header",
			configKourer: config.Kourier{
				DisableEnvoyServerHeader: true,
			},
			wantedServerHeaderTransformation: hcm.HttpConnectionManager_PASS_THROUGH,
		},
		{
			name: "test allow envoy server header",
			configKourer: config.Kourier{
				DisableEnvoyServerHeader: false,
			},
			wantedServerHeaderTransformation: hcm.HttpConnectionManager_OVERWRITE,
		},
		{
			name:                             "test allow envoy server header, no setting",
			configKourer:                     config.Kourier{},
			wantedServerHeaderTransformation: hcm.HttpConnectionManager_OVERWRITE,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connManager := NewHTTPConnectionManager("test", &test.configKourer)
			assert.Equal(t, test.wantedServerHeaderTransformation, connManager.ServerHeaderTransformation)
		})
	}
}
