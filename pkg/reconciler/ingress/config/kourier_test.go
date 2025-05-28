/*
Copyright 2021 The Knative Authors

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
	"testing"

	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	_ "knative.dev/pkg/system/testing"
)

func TestKourierConfig(t *testing.T) {
	configTests := []struct {
		name    string
		wantErr bool
		want    *Kourier
		data    map[string]string
	}{{
		name: "default configuration",
		want: defaultKourierConfig(),
		data: map[string]string{},
	}, {
		name: "disable logging",
		want: &Kourier{
			EnableServiceAccessLogging: false,
			IdleTimeout:                0 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
		},
	}, {
		name:    "not a bool for logging",
		wantErr: true,
		data: map[string]string{
			enableServiceAccessLoggingKey: "foo",
		},
	}, {
		name: "enable proxy protocol, logging and internal cert",
		want: &Kourier{
			EnableServiceAccessLogging: true,
			EnableProxyProtocol:        true,
			ClusterCertSecret:          "my-cert",
			IdleTimeout:                0 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "true",
			enableProxyProtocol:           "true",
			clusterCert:                   "my-cert",
		},
	}, {
		name: "enable proxy protocol and disable logging, empty internal cert",
		want: &Kourier{
			EnableServiceAccessLogging: false,
			EnableProxyProtocol:        true,
			ClusterCertSecret:          "",
			IdleTimeout:                0 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
			enableProxyProtocol:           "true",
			clusterCert:                   "",
		},
	}, {
		name:    "not a bool for proxy protocol",
		wantErr: true,
		data: map[string]string{
			enableProxyProtocol: "foo",
		},
	}, {
		name: "set cipher suites",
		want: &Kourier{
			EnableServiceAccessLogging: false,
			CipherSuites:               sets.New("foo", "bar"),
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
			cipherSuites:                  "foo, bar",
		},
	}, {
		name: "set timeout to 200",
		want: &Kourier{
			EnableServiceAccessLogging: true,
			EnableProxyProtocol:        false,
			ClusterCertSecret:          "",
			IdleTimeout:                200 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "true",
			enableProxyProtocol:           "false",
			clusterCert:                   "",
			IdleTimeoutKey:                "200s",
		},
	}, {
		name: "add 3 trusted hops",
		want: &Kourier{
			EnableServiceAccessLogging: false,
			TrustedHopsCount:           3,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
			trustedHopsCount:              "3",
		},
	}, {
		name: "configure tracing",
		want: &Kourier{
			EnableServiceAccessLogging: true,
			Tracing: Tracing{
				Enabled:           true,
				CollectorHost:     "jaeger.default.svc.cluster.local",
				CollectorPort:     9411,
				CollectorEndpoint: "/api/v2/spans",
			},
		},
		data: map[string]string{
			TracingCollectorFullEndpoint: "jaeger.default.svc.cluster.local:9411/api/v2/spans",
		},
	}, {
		name: "do not enable tracing",
		want: &Kourier{
			EnableServiceAccessLogging: true,
			Tracing: Tracing{
				Enabled: false,
			},
		},
		data: map[string]string{
			TracingCollectorFullEndpoint: "",
		},
	}, {
		name: "Enable use remote address",
		want: &Kourier{
			EnableServiceAccessLogging: true,
			UseRemoteAddress:           true,
		},
		data: map[string]string{
			useRemoteAddress: "true",
		},
	}}

	for _, tt := range configTests {
		t.Run(tt.name, func(t *testing.T) {
			actualCM, err := NewKourierConfigFromConfigMap(&corev1.ConfigMap{
				Data: tt.data,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfigFromConfigMap() error = %v, WantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(actualCM, tt.want); diff != "" {
				t.Errorf("Config mismatch: diff(-want,+got):\n%s", diff)
			}

			actualCfg, err := NewKourierConfigFromMap(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfigFromMap() error = %v, WantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(actualCfg, actualCM); diff != "" {
				t.Errorf("Config mismatch: diff(-want,+got):\n%s", diff)
			}
		})
	}
}

func TestAsExternalAuthz(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]string
		want    ExternalAuthz
		wantErr bool
	}{{
		name: "basic config",
		data: map[string]string{
			extauthzHostKey:     "auth.default.svc.cluster.local:9000",
			extauthzProtocolKey: "grpc",
		},
		want: ExternalAuthz{
			Enabled: true,
			Config: ExternalAuthzConfig{
				Host:            "auth.default.svc.cluster.local",
				Port:            9000,
				MaxRequestBytes: 8192,
				Timeout:         2000,
				Protocol:        "grpc",
			},
		},
	}, {
		name: "full config",
		data: map[string]string{
			extauthzHostKey:                "auth.default.svc.cluster.local:9000",
			extauthzProtocolKey:            "grpc",
			extauthzFailureModeAllowKey:    "true",
			extauthzMaxRequestBodyBytesKey: "1024",
			extauthzTimeoutKey:             "2",
			extauthzPathPrefixKey:          "/check",
			extauthzPackAsBytesKey:         "true",
		},
		want: ExternalAuthz{
			Enabled: true,
			Config: ExternalAuthzConfig{
				Host:             "auth.default.svc.cluster.local",
				Port:             9000,
				Protocol:         "grpc",
				FailureModeAllow: true,
				MaxRequestBytes:  1024,
				Timeout:          2,
				PathPrefix:       "/check",
				PackAsBytes:      true,
			},
		},
	}, {
		name: "failed to parse config",
		data: map[string]string{
			extauthzHostKey:             "auth.default.svc.cluster.local:9000",
			extauthzProtocolKey:         "grpc",
			extauthzFailureModeAllowKey: "invalid",
		},
		wantErr: true,
	}, {
		name: "invalid protocol",
		data: map[string]string{
			extauthzHostKey:     "auth.default.svc.cluster.local:9000",
			extauthzProtocolKey: "invalid",
		},
		wantErr: true,
	}, {
		name: "invalid port",
		data: map[string]string{
			extauthzHostKey:     "auth.default.svc.cluster.local:invalid",
			extauthzProtocolKey: "grpc",
		},
		wantErr: true,
	}, {
		name: "port above unixMaxPort",
		data: map[string]string{
			extauthzHostKey:     "auth.default.svc.cluster.local:99999",
			extauthzProtocolKey: "grpc",
		},
		wantErr: true,
	}, {
		name: "empty port",
		data: map[string]string{
			extauthzHostKey:     "auth.default.svc.cluster.local",
			extauthzProtocolKey: "grpc",
		},
		wantErr: true,
	}, {
		name: "empty host",
		data: map[string]string{},
		want: ExternalAuthz{
			Enabled: false,
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ExternalAuthz
			err := asExternalAuthz(&got)(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("asExternalAuthz() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("asExternalAuthz() diff(-want,+got):\n%s", diff)
				}
			}
		})
	}
}
