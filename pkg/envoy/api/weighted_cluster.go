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
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes/wrappers"
)

// NewWeightedCluster creates a new WeightedCluster.
func NewWeightedCluster(name string, trafficPerc uint32, headers map[string]string) *route.WeightedCluster_ClusterWeight {
	return &route.WeightedCluster_ClusterWeight{
		Name: name,
		Weight: &wrappers.UInt32Value{
			Value: trafficPerc,
		},
		RequestHeadersToAdd: headersToAdd(headers),
	}
}
