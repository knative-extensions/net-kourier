package envoy

import (
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func NewWeightedCluster(name string, trafficPerc uint32, headers map[string]string) route.WeightedCluster_ClusterWeight {
	return route.WeightedCluster_ClusterWeight{
		Name: name,
		Weight: &wrappers.UInt32Value{
			Value: trafficPerc,
		},
		RequestHeadersToAdd: headersToAdd(headers),
	}
}
