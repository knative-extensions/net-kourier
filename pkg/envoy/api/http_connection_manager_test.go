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
	"testing"
	"time"

	envoy_config_filter_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	fileaccesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gotest.tools/v3/assert"
	"knative.dev/net-kourier/pkg/config"
)

func TestNewHTTPConnectionManagerWithoutAccessLogWithoutProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: false,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
	}
	connManager := NewHTTPConnectionManager("test", &kourierConfig)
	assert.Check(t, len(connManager.AccessLog) == 0)
	assert.Check(t, connManager.UseRemoteAddress == nil)
}

func TestNewHTTPConnectionManagerWithAccessLogWithoutProxyProtocol(t *testing.T) {
	kourierConfig := config.Kourier{
		EnableServiceAccessLogging: true,
		EnableProxyProtocol:        false,
		IdleTimeout:                0 * time.Second,
	}
	connManager := NewHTTPConnectionManager("test", &kourierConfig)
	assert.Check(t, connManager.UseRemoteAddress == nil)
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
