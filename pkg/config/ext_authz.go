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

package config

import (
	"net"
	"os"
	"strconv"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/kelseyhightower/envconfig"
)

var ExternalAuthz = &ExternalAuthzConfig{}

type ExternalAuthzConfig struct {
	Enabled    bool
	Cluster    *v2.Cluster
	HTTPFilter *httpconnectionmanagerv2.HttpFilter
}

type config struct {
	Host             string `split_words:"true"`
	FailureModeAllow bool   `split_words:"true"`
	MaxRequestBytes  uint32 `split_words:"true" default:"8192"`
	Timeout          int    `split_words:"true" default:"2000"`
}

func init() {
	if _, ok := os.LookupEnv("KOURIER_EXTAUTHZ_HOST"); !ok {
		// No ExtAuthz setup.
		return
	}

	var env config
	envconfig.MustProcess("KOURIER_EXTAUTHZ", &env)

	host, portStr, err := net.SplitHostPort(env.Host)
	if err != nil {
		panic(err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(err)
	}

	timeout := time.Duration(env.Timeout) * time.Millisecond

	ExternalAuthz = &ExternalAuthzConfig{
		Enabled:    true,
		Cluster:    extAuthzCluster(host, uint32(port)),
		HTTPFilter: externalAuthZFilter(ExternalAuthzCluster, timeout, env.FailureModeAllow, env.MaxRequestBytes),
	}
}

func extAuthzCluster(host string, port uint32) *v2.Cluster {
	return &v2.Cluster{
		Name: ExternalAuthzCluster,
		ClusterDiscoveryType: &v2.Cluster_Type{
			Type: v2.Cluster_STRICT_DNS,
		},
		ConnectTimeout: ptypes.DurationProto(5 * time.Second),
		LoadAssignment: &v2.ClusterLoadAssignment{
			ClusterName: ExternalAuthzCluster,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{&endpoint.LbEndpoint{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_SocketAddress{
									SocketAddress: &core.SocketAddress{
										Protocol: core.SocketAddress_TCP,
										Address:  host,
										PortSpecifier: &core.SocketAddress_PortValue{
											PortValue: port,
										},
										Ipv4Compat: true,
									},
								},
							},
						},
					},
				}},
			}},
		},
	}
}

func externalAuthZFilter(clusterName string, timeout time.Duration, failureModeAllow bool, maxRequestBytes uint32) *httpconnectionmanagerv2.HttpFilter {
	extAuthConfig := &extAuthService.ExtAuthz{
		Services: &extAuthService.ExtAuthz_GrpcService{
			GrpcService: &envoy_api_v2_core.GrpcService{
				TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
						ClusterName: clusterName,
					},
				},
				Timeout: ptypes.DurationProto(timeout),
				InitialMetadata: []*envoy_api_v2_core.HeaderValue{{
					Key:   "client",
					Value: "kourier",
				}},
			},
		},
		FailureModeAllow: failureModeAllow,
		WithRequestBody: &extAuthService.BufferSettings{
			MaxRequestBytes:     maxRequestBytes,
			AllowPartialMessage: true,
		},
		ClearRouteCache: false,
	}

	envoyConf, err := conversion.MessageToStruct(extAuthConfig)
	if err != nil {
		panic(err)
	}

	return &httpconnectionmanagerv2.HttpFilter{
		Name: wellknown.HTTPExternalAuthorization,
		ConfigType: &httpconnectionmanagerv2.HttpFilter_Config{
			Config: envoyConf,
		},
	}
}
