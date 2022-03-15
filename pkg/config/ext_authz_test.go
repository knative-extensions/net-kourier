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
	"reflect"
	"testing"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	httpOptions "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

func Test_isValidProtocol(t *testing.T) {
	tests := []struct {
		protocol extAuthzProtocol
		want     bool
	}{
		{protocol: "grpc", want: true},
		{protocol: "http", want: true},
		{protocol: "https", want: true},
		{protocol: "", want: false},
		{protocol: "unknown", want: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if got := isValidExtAuthzProtocol(tt.protocol); got != tt.want {
				t.Errorf("isValidExtAuthzProtocol() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_extAuthzCluster_httpProtocolOptions(t *testing.T) {
	type args struct {
		host     string
		port     uint32
		protocol extAuthzProtocol
	}
	tests := []struct {
		name                      string
		args                      args
		httpProtocolOptionsWanted *httpOptions.HttpProtocolOptions
	}{{
		name: "grpc",
		args: args{
			host:     "example.com",
			port:     50051,
			protocol: "grpc",
		},
		httpProtocolOptionsWanted: &httpOptions.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
				},
			},
		},
	}, {
		name: "http",
		args: args{
			host:     "example.com",
			port:     8080,
			protocol: "http",
		},
		httpProtocolOptionsWanted: &httpOptions.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
				},
			},
		},
	}, {
		name: "https",
		args: args{
			host:     "example.com",
			port:     8443,
			protocol: "https",
		},
		httpProtocolOptionsWanted: &httpOptions.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
				},
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extAuthzCluster(tt.args.host, tt.args.port, tt.args.protocol)
			httpProtocolOptionsGot, ok := got.TypedExtensionProtocolOptions[extAuthzClusterTypedExtensionProtocolOptionsHTTP]

			if !ok {
				t.Errorf("Cannot found %s in config", extAuthzClusterTypedExtensionProtocolOptionsHTTP)
			}

			httpProtocolOptionsWantedAny, err := anypb.New(tt.httpProtocolOptionsWanted)
			if err != nil {
				t.Errorf("Cannot convert HttpProtocolOptions to Any")
			}

			if !reflect.DeepEqual(httpProtocolOptionsGot, httpProtocolOptionsWantedAny) {
				t.Errorf("extAuthzCluster() http protocol options = %v, want %v", httpProtocolOptionsGot, httpProtocolOptionsWantedAny)
			}
		})
	}
}

func Test_externalAuthZFilter_extAuthz(t *testing.T) {
	tests := []struct {
		name           string
		conf           *config
		extAuthzWanted *extAuthService.ExtAuthz
	}{{
		name: "grpc",
		conf: &config{
			Host:            "example.com:50051",
			MaxRequestBytes: 8192,
			Timeout:         2000,
			Protocol:        "grpc",
		},
		extAuthzWanted: &extAuthService.ExtAuthz{
			TransportApiVersion: core.ApiVersion_V3,
			WithRequestBody: &extAuthService.BufferSettings{
				MaxRequestBytes:     8192,
				AllowPartialMessage: true,
			},
			Services: &extAuthService.ExtAuthz_GrpcService{
				GrpcService: &core.GrpcService{
					TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
							ClusterName: extAuthzClusterName,
						},
					},
					Timeout: durationpb.New(time.Duration(2000) * time.Millisecond),
					InitialMetadata: []*core.HeaderValue{{
						Key:   "client",
						Value: "kourier",
					}},
				},
			},
		},
	}, {
		name: "http",
		conf: &config{
			Host:            "example.com:8080",
			MaxRequestBytes: 8192,
			Timeout:         2000,
			Protocol:        "http",
		},
		extAuthzWanted: &extAuthService.ExtAuthz{
			TransportApiVersion: core.ApiVersion_V3,
			WithRequestBody: &extAuthService.BufferSettings{
				MaxRequestBytes:     8192,
				AllowPartialMessage: true,
			},
			Services: &extAuthService.ExtAuthz_HttpService{
				HttpService: &extAuthService.HttpService{
					ServerUri: &core.HttpUri{
						Uri: "http://example.com:8080",
						HttpUpstreamType: &core.HttpUri_Cluster{
							Cluster: extAuthzClusterName,
						},
						Timeout: durationpb.New(time.Duration(2000) * time.Millisecond),
					},
					AuthorizationRequest: &extAuthService.AuthorizationRequest{
						HeadersToAdd: []*core.HeaderValue{{
							Key:   "client",
							Value: "kourier",
						}},
					},
				},
			},
		},
	}, {
		name: "http with path prefix",
		conf: &config{
			Host:            "example.com:8080",
			MaxRequestBytes: 8192,
			Timeout:         2000,
			Protocol:        "http",
			PathPrefix:      "/verify",
		},
		extAuthzWanted: &extAuthService.ExtAuthz{
			TransportApiVersion: core.ApiVersion_V3,
			WithRequestBody: &extAuthService.BufferSettings{
				MaxRequestBytes:     8192,
				AllowPartialMessage: true,
			},
			Services: &extAuthService.ExtAuthz_HttpService{
				HttpService: &extAuthService.HttpService{
					ServerUri: &core.HttpUri{
						Uri: "http://example.com:8080",
						HttpUpstreamType: &core.HttpUri_Cluster{
							Cluster: extAuthzClusterName,
						},
						Timeout: durationpb.New(time.Duration(2000) * time.Millisecond),
					},
					PathPrefix: "/verify",
					AuthorizationRequest: &extAuthService.AuthorizationRequest{
						HeadersToAdd: []*core.HeaderValue{{
							Key:   "client",
							Value: "kourier",
						}},
					},
				},
			},
		},
	}, {
		name: "https",
		conf: &config{
			Host:            "example.com:8443",
			MaxRequestBytes: 8192,
			Timeout:         2000,
			Protocol:        "https",
		},
		extAuthzWanted: &extAuthService.ExtAuthz{
			TransportApiVersion: core.ApiVersion_V3,
			WithRequestBody: &extAuthService.BufferSettings{
				MaxRequestBytes:     8192,
				AllowPartialMessage: true,
			},
			Services: &extAuthService.ExtAuthz_HttpService{
				HttpService: &extAuthService.HttpService{
					ServerUri: &core.HttpUri{
						Uri: "https://example.com:8443",
						HttpUpstreamType: &core.HttpUri_Cluster{
							Cluster: extAuthzClusterName,
						},
						Timeout: durationpb.New(time.Duration(2000) * time.Millisecond),
					},
					AuthorizationRequest: &extAuthService.AuthorizationRequest{
						HeadersToAdd: []*core.HeaderValue{{
							Key:   "client",
							Value: "kourier",
						}},
					},
				},
			},
		},
	}, {
		name: "https with path prefix",
		conf: &config{
			Host:            "example.com:8443",
			MaxRequestBytes: 8192,
			Timeout:         2000,
			Protocol:        "https",
			PathPrefix:      "/verify",
		},
		extAuthzWanted: &extAuthService.ExtAuthz{
			TransportApiVersion: core.ApiVersion_V3,
			WithRequestBody: &extAuthService.BufferSettings{
				MaxRequestBytes:     8192,
				AllowPartialMessage: true,
			},
			Services: &extAuthService.ExtAuthz_HttpService{
				HttpService: &extAuthService.HttpService{
					ServerUri: &core.HttpUri{
						Uri: "https://example.com:8443",
						HttpUpstreamType: &core.HttpUri_Cluster{
							Cluster: extAuthzClusterName,
						},
						Timeout: durationpb.New(time.Duration(2000) * time.Millisecond),
					},
					PathPrefix: "/verify",
					AuthorizationRequest: &extAuthService.AuthorizationRequest{
						HeadersToAdd: []*core.HeaderValue{{
							Key:   "client",
							Value: "kourier",
						}},
					},
				},
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := externalAuthZFilter(tt.conf)

			extAuthzWantedAny, err := anypb.New(tt.extAuthzWanted)
			if err != nil {
				t.Errorf("Cannot convert ExtAuthz to Any")
			}

			if !reflect.DeepEqual(got.GetTypedConfig(), extAuthzWantedAny) {
				t.Errorf("externalAuthZFilter() extAuthz = %v, want %v", got.GetTypedConfig(), extAuthzWantedAny)
			}
		})
	}
}
