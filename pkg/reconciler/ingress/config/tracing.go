/*
Copyright 2025 The Knative Authors

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

package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_trace_v3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	httpOptions "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"knative.dev/pkg/observability/metrics"
)

const (
	DefaultTracingProtocol     = metrics.ProtocolGRPC
	DefaultTracingSamplingRate = 1.0
	DefaultTracingServiceName  = "kourier-knative"
	OtelCollectorClusterName   = "otel-collector"
)

type Tracing struct {
	Enabled bool

	// OTLP fields
	Endpoint     string
	Protocol     string
	SamplingRate float64
	ServiceName  string

	// Parsed OTLP endpoint components
	OTLPHost string
	OTLPPort uint32
	OTLPPath string
}

// Cluster returns the Envoy cluster configuration for tracing.
// Returns nil if tracing is not enabled.
func (t *Tracing) Cluster() *envoyclusterv3.Cluster {
	if !t.Enabled {
		return nil
	}

	otelCollectorCluster := &envoyclusterv3.Cluster{
		Name:                 OtelCollectorClusterName,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS},
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: OtelCollectorClusterName,
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: []*endpoint.LbEndpoint{
						{
							HostIdentifier: &endpoint.LbEndpoint_Endpoint{
								Endpoint: &endpoint.Endpoint{
									Address: &core.Address{
										Address: &core.Address_SocketAddress{
											SocketAddress: &core.SocketAddress{
												Protocol: core.SocketAddress_TCP,
												Address:  t.OTLPHost,
												PortSpecifier: &core.SocketAddress_PortValue{
													PortValue: t.OTLPPort,
												},
												Ipv4Compat: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if t.Protocol == metrics.ProtocolGRPC {
		h2Options, _ := anypb.New(&httpOptions.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
				},
			},
		})
		otelCollectorCluster.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": h2Options,
		}
	}

	return otelCollectorCluster
}

// TracingConfig returns the HttpConnectionManager tracing configuration.
// Returns nil if tracing is not enabled.
func (t *Tracing) TracingConfig() *hcm.HttpConnectionManager_Tracing {
	if !t.Enabled {
		return nil
	}

	const otelTracerName = "envoy.tracers.opentelemetry"

	var otelConfig *anypb.Any
	if t.Protocol == metrics.ProtocolHTTPProtobuf {
		// HTTP/protobuf protocol
		otelConfig, _ = anypb.New(&envoy_config_trace_v3.OpenTelemetryConfig{
			HttpService: &core.HttpService{
				HttpUri: &core.HttpUri{
					Uri: t.Endpoint,
					HttpUpstreamType: &core.HttpUri_Cluster{
						Cluster: OtelCollectorClusterName,
					},
					Timeout: durationpb.New(5 * time.Second),
				},
			},
			ServiceName: t.ServiceName,
		})
	} else {
		// gRPC protocol (requires HTTP/2 on otel-collector cluster)
		otelConfig, _ = anypb.New(&envoy_config_trace_v3.OpenTelemetryConfig{
			GrpcService: &core.GrpcService{
				TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
						ClusterName: OtelCollectorClusterName,
					},
				},
				Timeout: durationpb.New(250 * time.Millisecond),
			},
			ServiceName: t.ServiceName,
		})
	}

	return &hcm.HttpConnectionManager_Tracing{
		Provider: &envoy_config_trace_v3.Tracing_Http{
			Name: otelTracerName,
			ConfigType: &envoy_config_trace_v3.Tracing_Http_TypedConfig{
				TypedConfig: otelConfig,
			},
		},
		OverallSampling: &envoy_type_v3.Percent{
			Value: t.SamplingRate * 100,
		},
	}
}

// parseOtlpEndpoint parses an OTLP endpoint URL and extracts host, port and path.
func parseOtlpEndpoint(endpoint string) (host string, port uint32, path string, err error) {
	u, parseErr := url.Parse(endpoint)
	if parseErr != nil {
		return "", 0, "", fmt.Errorf("invalid URL: %w", parseErr)
	}

	host = u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		return "", 0, "", errors.New("port not specified in URL")
	}

	portInt, parseErr := strconv.ParseUint(portStr, 10, 32)
	if parseErr != nil {
		return "", 0, "", fmt.Errorf("invalid port: %w", parseErr)
	}

	return host, uint32(portInt), u.Path, nil
}
