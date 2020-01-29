package envoy

import (
	"testing"
	"time"

	"gotest.tools/assert"

	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
)

func TestNewCluster(t *testing.T) {
	name := "myTestCluster_12345"
	connectTimeout := 5 * time.Second

	endpoint1 := NewLBEndpoint("127.0.0.1", 1234)
	endpoint2 := NewLBEndpoint("127.0.0.2", 1234)
	endpoints := []*endpoint.LbEndpoint{endpoint1, endpoint2}

	c := NewCluster(name, connectTimeout, endpoints, true)

	assert.Equal(t, c.GetConnectTimeout().Seconds, int64(connectTimeout.Seconds()))
	assert.Assert(t, c.Http2ProtocolOptions != nil)
	assert.Equal(t, c.GetName(), name)
	assert.DeepEqual(t, c.LoadAssignment.Endpoints[0].LbEndpoints, endpoints)

	c = NewCluster(name, connectTimeout, endpoints, false)

	assert.Assert(t, c.GetHttp2ProtocolOptions() == nil)
}
