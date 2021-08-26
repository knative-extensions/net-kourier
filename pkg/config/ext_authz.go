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
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	v3Cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/kelseyhightower/envconfig"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	extAuthzClusterName = "extAuthz"
	// See https://en.wikipedia.org/wiki/Registered_port.
	unixMaxPort = 65535
)

// ExternalAuthz is the configuration of external authorization.
var ExternalAuthz = &ExternalAuthzConfig{
	Enabled: false,
}

// ExternalAuthzConfig specifies parameters for external authorization configuration.
type ExternalAuthzConfig struct {
	Enabled    bool
	Cluster    *v3Cluster.Cluster
	HTTPFilter *hcm.HttpFilter
}

type config struct {
	Host             string `split_words:"true"`
	FailureModeAllow bool   `split_words:"true"`
	MaxRequestBytes  uint32 `split_words:"true" default:"8192"`
	Timeout          int    `split_words:"true" default:"2000"`
}

func init() {
	if host := os.Getenv("KOURIER_EXTAUTHZ_HOST"); host == "" {
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

	if port > unixMaxPort {
		// Bail out if we exceed the maximum port number.
		panic(fmt.Sprintf("port %d bigger than %d", port, unixMaxPort))
	}

	timeout := time.Duration(env.Timeout) * time.Millisecond

	ExternalAuthz = &ExternalAuthzConfig{
		Enabled:    true,
		Cluster:    extAuthzCluster(host, uint32(port)),
		HTTPFilter: externalAuthZFilter(extAuthzClusterName, timeout, env.FailureModeAllow, env.MaxRequestBytes),
	}
}

func extAuthzCluster(host string, port uint32) *v3Cluster.Cluster {
	return &v3Cluster.Cluster{
		Name: extAuthzClusterName,
		ClusterDiscoveryType: &v3Cluster.Cluster_Type{
			Type: v3Cluster.Cluster_STRICT_DNS,
		},
		//nolint: staticcheck // TODO: Http2ProtocolOptions is deprecated.
		Http2ProtocolOptions: &core.Http2ProtocolOptions{},
		ConnectTimeout:       durationpb.New(5 * time.Second),
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: extAuthzClusterName,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
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

func externalAuthZFilter(clusterName string, timeout time.Duration, failureModeAllow bool, maxRequestBytes uint32) *hcm.HttpFilter {
	extAuthConfig := &extAuthService.ExtAuthz{
		Services: &extAuthService.ExtAuthz_GrpcService{
			GrpcService: &core.GrpcService{
				TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
						ClusterName: clusterName,
					},
				},
				Timeout: durationpb.New(timeout),
				InitialMetadata: []*core.HeaderValue{{
					Key:   "client",
					Value: "kourier",
				}},
			},
		},
		TransportApiVersion: core.ApiVersion_V3,
		FailureModeAllow:    failureModeAllow,
		WithRequestBody: &extAuthService.BufferSettings{
			MaxRequestBytes:     maxRequestBytes,
			AllowPartialMessage: true,
		},
		ClearRouteCache: false,
	}

	envoyConf, err := anypb.New(extAuthConfig)
	if err != nil {
		panic(err)
	}

	return &hcm.HttpFilter{
		Name: wellknown.HTTPExternalAuthorization,
		ConfigType: &hcm.HttpFilter_TypedConfig{
			TypedConfig: envoyConf,
		},
	}
}
