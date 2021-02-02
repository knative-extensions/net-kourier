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

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	envoyCluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	upstreams "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
)

// NewCluster generates a new v3.Cluster with the given settings.
func NewCluster(
	name string,
	connectTimeout time.Duration,
	endpoints []*endpoint.LbEndpoint,
	isHTTP2 bool,
	discoveryType envoyCluster.Cluster_DiscoveryType) *envoyCluster.Cluster {

	cluster := &envoyCluster.Cluster{
		Name: name,
		ClusterDiscoveryType: &envoyCluster.Cluster_Type{
			Type: discoveryType,
		},
		ConnectTimeout: ptypes.DurationProto(connectTimeout),
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: endpoints,
			}},
		},
	}

	if isHTTP2 {
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
		//cluster.TypedExtensionProtocolOptions = http2ProtocolOptions()
	}

	return cluster
}

func http2ProtocolOptions() map[string]*any.Any {
	return map[string]*any.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMarshalAny(
			&upstreams.HttpProtocolOptions{
				UpstreamProtocolOptions: &upstreams.HttpProtocolOptions_ExplicitHttpConfig_{
					ExplicitHttpConfig: &upstreams.HttpProtocolOptions_ExplicitHttpConfig{
						ProtocolConfig: &upstreams.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
					},
				},
			}),
	}
}

func mustMarshalAny(pb proto.Message) *any.Any {
	out, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err)
	}
	return out
}
