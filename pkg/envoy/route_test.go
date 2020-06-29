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
	"net/http"
	"testing"
	"time"

	"gotest.tools/assert"

	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
)

func TestNewRouteWithRetry(t *testing.T) {
	name := "testRoute_12345"
	path := "/my_route"
	routeTimeout := 5 * time.Second
	attempts := 5
	perTryTimeout := 1 * time.Second

	headerName := "myNewHeader"
	headerValue := "Value"
	AppendHeaders := map[string]string{
		headerName: headerValue,
	}

	var wrs []*envoy_api_v2_route.WeightedCluster_ClusterWeight
	r := NewRoute(name, nil, path, wrs, routeTimeout, uint32(attempts), perTryTimeout, AppendHeaders)

	assert.Equal(t, r.Name, name)
	assert.Equal(t, r.RequestHeadersToAdd[0].GetHeader().GetKey(), headerName)
	assert.Equal(t, r.RequestHeadersToAdd[0].GetHeader().GetValue(), headerValue)
	assert.Equal(t, r.GetRoute().GetUpgradeConfigs()[0].UpgradeType, "websocket")
	assert.Equal(t, r.Match.GetPrefix(), path)
	assert.Equal(t, r.GetRoute().RetryPolicy.RetryOn, "5xx")
	assert.Equal(t, r.GetRoute().RetryPolicy.NumRetries.Value, uint32(attempts))

}

func TestNewRouteWithoutRetry(t *testing.T) {

	name := "testRoute_12345"
	path := "/my_route"
	AppendHeaders := map[string]string{}
	var wrs []*envoy_api_v2_route.WeightedCluster_ClusterWeight

	r := NewRoute(name, nil, path, wrs, 0, uint32(0), 0, AppendHeaders)
	assert.Assert(t, r.GetRoute().RetryPolicy == nil)
}

func TestNewRouteHeaderMatch(t *testing.T) {

	name := "testRoute_12345"
	path := "/my_route"
	headerMatch := []*envoy_api_v2_route.HeaderMatcher{
		{
			Name: "myHeader",
			HeaderMatchSpecifier: &envoy_api_v2_route.HeaderMatcher_ExactMatch{
				ExactMatch: "strict",
			},
		},
	}
	AppendHeaders := map[string]string{}
	var wrs []*envoy_api_v2_route.WeightedCluster_ClusterWeight

	r := NewRoute(name, headerMatch, path, wrs, 0, uint32(0), 0, AppendHeaders)
	assert.Equal(t, r.Match.Headers[0].Name, "myHeader")
	assert.Equal(t, r.Match.Headers[0].GetExactMatch(), "strict")

}

func TestNewRouteStatusOK(t *testing.T) {

	name := "testRoute_12345"
	path := "/my_route"

	r := NewRouteStatusOK(name, path)

	assert.Equal(t, r.Match.GetPath(), path)
	assert.Equal(t, r.GetDirectResponse().Status, uint32(http.StatusOK))
}
