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
	"errors"
	"fmt"
	"time"

	v3Cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	httpOptions "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	extAuthzClusterName = "extAuthz"
	// See https://en.wikipedia.org/wiki/Registered_port.
	unixMaxPort = 65535
)

// ExternalAuthzConfig specifies parameters for external authorization configuration.
type ExternalAuthz struct {
	Enabled bool
	config  externalAuthzConfig
}

func (e *ExternalAuthz) Cluster() *v3Cluster.Cluster {
	return extAuthzCluster(e.config.Host, uint32(e.config.Port), e.config.Protocol)
}

func (e *ExternalAuthz) HTTPFilter() *hcm.HttpFilter {
	return externalAuthZFilter(&e.config)
}

type extAuthzProtocol string

const (
	extAuthzProtocolGRPC  extAuthzProtocol = "grpc"
	extAuthzProtocolHTTP  extAuthzProtocol = "http"
	extAuthzProtocolHTTPS extAuthzProtocol = "https"
)

var extAuthzProtocols = map[extAuthzProtocol]struct{}{
	extAuthzProtocolGRPC:  {},
	extAuthzProtocolHTTP:  {},
	extAuthzProtocolHTTPS: {},
}

func isValidExtAuthzProtocol(protocol extAuthzProtocol) bool {
	_, ok := extAuthzProtocols[protocol]
	return ok
}

type externalAuthzConfig struct {
	Host             string
	Port             int
	FailureModeAllow bool
	MaxRequestBytes  uint32           `default:"8192"`
	Timeout          int              `default:"2000"`
	Protocol         extAuthzProtocol `default:"grpc"`
	PackAsBytes      bool             `default:"false"`
	PathPrefix       string
}

const extAuthzClusterTypedExtensionProtocolOptionsHTTP = "envoy.extensions.upstreams.http.v3.HttpProtocolOptions"

func extAuthzCluster(host string, port uint32, protocol extAuthzProtocol) *v3Cluster.Cluster {
	var explicitHTTPConfig *httpOptions.HttpProtocolOptions_ExplicitHttpConfig

	switch protocol {
	case extAuthzProtocolGRPC:
		explicitHTTPConfig = &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
			ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
		}
	case extAuthzProtocolHTTP, extAuthzProtocolHTTPS:
		explicitHTTPConfig = &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
			ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
		}
	}

	opts, _ := anypb.New(&httpOptions.HttpProtocolOptions{
		UpstreamProtocolOptions: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: explicitHTTPConfig,
		},
	})

	return &v3Cluster.Cluster{
		Name: extAuthzClusterName,
		ClusterDiscoveryType: &v3Cluster.Cluster_Type{
			Type: v3Cluster.Cluster_STRICT_DNS,
		},
		TypedExtensionProtocolOptions: map[string]*anypb.Any{
			extAuthzClusterTypedExtensionProtocolOptionsHTTP: opts,
		},
		ConnectTimeout: durationpb.New(5 * time.Second),
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

var errPackAsBytesInvalidWithProtocolHTTP = errors.New("pack as bytes option cannot be set when using http protocol")

func externalAuthZFilter(conf *externalAuthzConfig) *hcm.HttpFilter {
	timeout := durationpb.New(time.Duration(conf.Timeout) * time.Millisecond)

	extAuthConfig := &extAuthService.ExtAuthz{
		TransportApiVersion: core.ApiVersion_V3,
		FailureModeAllow:    conf.FailureModeAllow,
		WithRequestBody: &extAuthService.BufferSettings{
			MaxRequestBytes:     conf.MaxRequestBytes,
			AllowPartialMessage: true,
		},
		ClearRouteCache: false,
	}

	if conf.Protocol != extAuthzProtocolGRPC && conf.PackAsBytes {
		panic(errPackAsBytesInvalidWithProtocolHTTP)
	}

	extAuthConfig.WithRequestBody.PackAsBytes = conf.PackAsBytes

	headers := []*core.HeaderValue{{
		Key:   "client",
		Value: "kourier",
	}}

	switch conf.Protocol {
	case extAuthzProtocolGRPC:
		extAuthConfig.Services = &extAuthService.ExtAuthz_GrpcService{
			GrpcService: &core.GrpcService{
				TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
						ClusterName: extAuthzClusterName,
					},
				},
				Timeout:         timeout,
				InitialMetadata: headers,
			},
		}
	case extAuthzProtocolHTTP, extAuthzProtocolHTTPS:
		extAuthConfig.Services = &extAuthService.ExtAuthz_HttpService{
			HttpService: &extAuthService.HttpService{
				ServerUri: &core.HttpUri{
					Uri: fmt.Sprintf("%s://%s:%d", conf.Protocol, conf.Host, conf.Port),
					HttpUpstreamType: &core.HttpUri_Cluster{
						Cluster: extAuthzClusterName,
					},
					Timeout: timeout,
				},
				PathPrefix: conf.PathPrefix,
				AuthorizationRequest: &extAuthService.AuthorizationRequest{
					HeadersToAdd: headers,
				},
			},
		}
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
