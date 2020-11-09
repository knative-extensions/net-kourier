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
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/golang/protobuf/ptypes"
)

// NewCluster generates a new v2.Cluster with the given settings.
func NewCluster(
	name string,
	connectTimeout time.Duration,
	endpoints []*endpoint.LbEndpoint,
	isHTTP2 bool,
	discoveryType v2.Cluster_DiscoveryType) *v2.Cluster {

	cluster := &v2.Cluster{
		Name: name,
		ClusterDiscoveryType: &v2.Cluster_Type{
			Type: discoveryType,
		},
		ConnectTimeout: ptypes.DurationProto(connectTimeout),
		LoadAssignment: &v2.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: endpoints,
			}},
		},
	}

	if isHTTP2 {
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}

	return cluster
}
