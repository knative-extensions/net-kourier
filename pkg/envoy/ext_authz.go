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
	"kourier/pkg/config"
	"net"
	"os"
	"strconv"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	extAuthService "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/ext_authz/v2"
	httpconnectionmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/duration"
)

const maxRequestBytesDefault = 8192

type ExternalAuthzConfig struct {
	Enabled          bool
	Host             string
	Port             int
	FailureModeAllow bool
	MaxRequestBytes  int
	Timeout          duration.Duration
}

func GetExternalAuthzConfig() ExternalAuthzConfig {
	res := ExternalAuthzConfig{}
	var err error

	if externalAuthzURI, ok := os.LookupEnv(config.ExtAuthzHostEnv); ok {
		var strPort string
		res.Host, strPort, err = net.SplitHostPort(externalAuthzURI)
		if err != nil {
			panic(err)
		}
		res.Port, err = strconv.Atoi(strPort)
		if err != nil {
			panic(err)
		}
		res.Enabled = true
	}

	if failureMode, ok := os.LookupEnv(config.ExtAuthzFailureModeEnv); ok {
		res.FailureModeAllow, err = strconv.ParseBool(failureMode)
		if err != nil {
			panic(err)
		}
	}

	if maxRequestBytes, ok := os.LookupEnv(config.ExtAuthzMaxRequestsBytes); ok {
		res.MaxRequestBytes, err = strconv.Atoi(maxRequestBytes)
		if err != nil {
			panic(err)
		}
	} else {
		res.MaxRequestBytes = maxRequestBytesDefault
	}

	if strTimeout, ok := os.LookupEnv(config.ExtAuthzTimeout); ok {
		intTimeout, err := strconv.Atoi(strTimeout)
		if err != nil {
			panic(err)
		}

		nsTimeout := int32(intTimeout * 1000000)
		seconds := int64(nsTimeout / 1000000000)
		nanos := nsTimeout % 1000000000

		res.Timeout = duration.Duration{
			Seconds: seconds,
			Nanos:   nanos,
		}
	} else {
		res.Timeout = duration.Duration{
			Seconds: 2,
		}
	}

	return res
}

func (e *ExternalAuthzConfig) GetExtAuthzCluster() *v2.Cluster {
	endpoints := []*endpoint.LbEndpoint{NewLBEndpoint(e.Host, uint32(e.Port))}
	return NewCluster(config.ExternalAuthzCluster, 5*time.Second, endpoints,
		true, v2.Cluster_STRICT_DNS)
}

func (e *ExternalAuthzConfig) GetExternalAuthZFilter(clusterName string) httpconnectionmanagerv2.HttpFilter {

	extAuthConfig := extAuthService.ExtAuthz{
		Services: &extAuthService.ExtAuthz_GrpcService{
			GrpcService: &envoy_api_v2_core.GrpcService{
				TargetSpecifier: &envoy_api_v2_core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoy_api_v2_core.GrpcService_EnvoyGrpc{
						ClusterName: clusterName,
					},
				},
				Timeout: &e.Timeout,
				InitialMetadata: []*envoy_api_v2_core.HeaderValue{{
					Key:   "client",
					Value: "kourier",
				}},
			},
		},
		FailureModeAllow: e.FailureModeAllow,
		WithRequestBody: &extAuthService.BufferSettings{
			MaxRequestBytes:     uint32(e.MaxRequestBytes),
			AllowPartialMessage: true,
		},
		ClearRouteCache: false,
	}

	envoyConf, err := conversion.MessageToStruct(&extAuthConfig)
	if err != nil {
		panic(err)
	}

	extAuthzFilter := httpconnectionmanagerv2.HttpFilter{
		Name: wellknown.HTTPExternalAuthorization,
		ConfigType: &httpconnectionmanagerv2.HttpFilter_Config{
			Config: envoyConf,
		},
	}

	return extAuthzFilter
}
