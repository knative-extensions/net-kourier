package envoy

import (
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/golang/protobuf/ptypes"
)

func NewCluster(name string, connectTimeout time.Duration, endpoints []*endpoint.LbEndpoint,
	isHTTP2 bool, discoveryType v2.Cluster_DiscoveryType) *v2.Cluster {

	cluster := &v2.Cluster{
		Name: name,
		ClusterDiscoveryType: &v2.Cluster_Type{
			Type: discoveryType,
		},
		ConnectTimeout: ptypes.DurationProto(connectTimeout),
		LoadAssignment: &v2.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: endpoints,
				},
			},
		},
	}

	if isHTTP2 {
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}

	return cluster
}
