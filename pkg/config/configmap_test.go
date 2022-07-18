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
		want: DefaultConfig(),
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
		name: "set isolation-traffic to port",
		want: &Kourier{
			EnableServiceAccessLogging: true,
			EnableProxyProtocol:        false,
			ClusterCertSecret:          "",
			IdleTimeout:                0 * time.Second,
			TrafficIsolation:           "port",
		},
		data: map[string]string{
			trafficIsolation: "port",
		},
	}}

	for _, tt := range configTests {
		t.Run(tt.name, func(t *testing.T) {
			actualCM, err := NewConfigFromConfigMap(&corev1.ConfigMap{
				Data: tt.data,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfigFromConfigMap() error = %v, WantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(actualCM, tt.want); diff != "" {
				t.Errorf("Config mismatch: diff(-want,+got):\n%s", diff)
			}

			actualCfg, err := NewConfigFromMap(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfigFromMap() error = %v, WantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(actualCfg, actualCM); diff != "" {
				t.Errorf("Config mismatch: diff(-want,+got):\n%s", diff)
			}
		})
	}
}
