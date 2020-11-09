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

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	envoy_config_filter_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	"github.com/golang/protobuf/ptypes"
	"gotest.tools/assert"
)

func TestNewHTTPConnectionManager(t *testing.T) {
	connManager := NewHTTPConnectionManager("test")
	accessLog := connManager.AccessLog[0]
	accessLogPathAny := accessLog.ConfigType.(*envoy_config_filter_accesslog_v2.AccessLog_TypedConfig).TypedConfig
	fileAccesLog := &accesslog_v2.FileAccessLog{}

	err := ptypes.UnmarshalAny(accessLogPathAny, fileAccesLog)
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
	want := &v2.RouteConfiguration{
		Name:         "test",
		VirtualHosts: []*route.VirtualHost{vhost},
	}

	assert.DeepEqual(t, got, want)
}
