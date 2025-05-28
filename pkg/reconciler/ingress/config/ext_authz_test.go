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
	tests := []struct {
		name                      string
		config                    ExternalAuthzConfig
		httpProtocolOptionsWanted *httpOptions.HttpProtocolOptions
	}{{
		name: "grpc",
		config: ExternalAuthzConfig{
			Host:     "example.com",
			Port:     50051,
			Protocol: "grpc",
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
		config: ExternalAuthzConfig{
			Host:     "example.com",
			Port:     8080,
			Protocol: "http",
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
		config: ExternalAuthzConfig{
			Host:     "example.com",
			Port:     8443,
			Protocol: "https",
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
			ea := &ExternalAuthz{
				Enabled: true,
				Config:  tt.config,
			}

			got := ea.Cluster()
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
		conf           *ExternalAuthzConfig
		extAuthzWanted *extAuthService.ExtAuthz
		panicErrWanted error
	}{{
		name: "grpc",
		conf: &ExternalAuthzConfig{
			Host:            "example.com",
			Port:            50051,
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
		name: "grpc with pack as bytes enabled",
		conf: &ExternalAuthzConfig{
			Host:            "example.com",
			Port:            50051,
			MaxRequestBytes: 8192,
			Timeout:         2000,
			Protocol:        "grpc",
			PackAsBytes:     true,
		},
		extAuthzWanted: &extAuthService.ExtAuthz{
			TransportApiVersion: core.ApiVersion_V3,
			WithRequestBody: &extAuthService.BufferSettings{
				MaxRequestBytes:     8192,
				AllowPartialMessage: true,
				PackAsBytes:         true,
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
		conf: &ExternalAuthzConfig{
			Host:            "example.com",
			Port:            8080,
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
		conf: &ExternalAuthzConfig{
			Host:            "example.com",
			Port:            8080,
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
		name: "http with pack as bytes enabled",
		conf: &ExternalAuthzConfig{
			Host:            "example.com",
			Port:            8080,
			MaxRequestBytes: 8192,
			Timeout:         2000,
			Protocol:        "http",
			PackAsBytes:     true,
		},
		panicErrWanted: errPackAsBytesInvalidWithProtocolHTTP,
	}, {
		name: "https",
		conf: &ExternalAuthzConfig{
			Host:            "example.com",
			Port:            8443,
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
		conf: &ExternalAuthzConfig{
			Host:            "example.com",
			Port:            8443,
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
			defer func() {
				if r := recover(); r != nil && !errors.Is(r.(error), tt.panicErrWanted) {
					t.Errorf("externalAuthZFilter() extAuthz have panicked with \"%v\", want \"%v\"", r, tt.panicErrWanted)
				}
			}()
			e := &ExternalAuthz{
				Enabled: true,
				Config:  *tt.conf,
			}

			got := e.HTTPFilter()

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
