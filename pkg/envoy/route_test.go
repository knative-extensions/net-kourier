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
	r := NewRoute(name, path, wrs, routeTimeout, uint32(attempts), perTryTimeout, AppendHeaders)

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

	r := NewRoute(name, path, wrs, 0, uint32(0), 0, AppendHeaders)
	assert.Assert(t, r.GetRoute().RetryPolicy == nil)
}

func TestNewRouteStatusOK(t *testing.T) {

	name := "testRoute_12345"
	path := "/my_route"

	r := NewRouteStatusOK(name, path)

	assert.Equal(t, r.Match.GetPath(), path)
	assert.Equal(t, r.GetDirectResponse().Status, uint32(http.StatusOK))
}
