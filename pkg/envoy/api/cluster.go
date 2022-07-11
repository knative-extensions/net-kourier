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
	"github.com/golang/protobuf/ptypes/wrappers"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	httpOptions "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

type ClusterConnectionOpts struct {
	MaxConnections     uint32
	MaxRequests        uint32
	MaxPendingRequests uint32
	MaxRetries         uint32
	ConnectTimeout     time.Duration
}

// NewCluster generates a new v3.Cluster with the given settings.
func NewCluster(
	name string,
	connectOpts ClusterConnectionOpts,
	endpoints []*endpoint.LbEndpoint,
	isHTTP2 bool, transportSocket *envoycorev3.TransportSocket,
	discoveryType envoyclusterv3.Cluster_DiscoveryType) *envoyclusterv3.Cluster {

	cluster := &envoyclusterv3.Cluster{
		Name: name,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: discoveryType,
		},
		ConnectTimeout: durationpb.New(connectOpts.ConnectTimeout),
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: endpoints,
			}},
		},
		TransportSocket: transportSocket,
	}

	if isHTTP2 {
		opts, _ := anypb.New(&httpOptions.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
				},
			},
		})

		cluster.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": opts,
		}
	}

	// Add default connection settings to prevent unnecessary fuses
	// 参考文档 https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/circuit_breaker.proto#envoy-v3-api-field-config-cluster-v3-circuitbreakers-thresholds-max-connections
	// Istio has a similar action
	defaultPriorityCircuitBreakersThresholds := envoyclusterv3.CircuitBreakers_Thresholds{
		Priority:           envoycorev3.RoutingPriority_DEFAULT,
		MaxConnections:     &wrappers.UInt32Value{Value: connectOpts.MaxConnections},
		MaxRequests:        &wrappers.UInt32Value{Value: connectOpts.MaxRequests},
		MaxPendingRequests: &wrappers.UInt32Value{Value: connectOpts.MaxPendingRequests},
		MaxRetries:         &wrappers.UInt32Value{Value: connectOpts.MaxRetries},
	} // default policy
	highPriorityCircuitBreakersThresholds := envoyclusterv3.CircuitBreakers_Thresholds{
		Priority:           envoycorev3.RoutingPriority_HIGH,
		MaxConnections:     &wrappers.UInt32Value{Value: connectOpts.MaxConnections},
		MaxRequests:        &wrappers.UInt32Value{Value: connectOpts.MaxRequests},
		MaxPendingRequests: &wrappers.UInt32Value{Value: connectOpts.MaxPendingRequests},
		MaxRetries:         &wrappers.UInt32Value{Value: connectOpts.MaxRetries},
	} // high level policy
	cluster.CircuitBreakers = &envoyclusterv3.CircuitBreakers{
		Thresholds: []*envoyclusterv3.CircuitBreakers_Thresholds{&defaultPriorityCircuitBreakersThresholds, &highPriorityCircuitBreakersThresholds},
	}

	return cluster
}
