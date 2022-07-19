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
	"time"

	corev1 "k8s.io/api/core/v1"

	cm "knative.dev/pkg/configmap"
)

// TrafficIsolationType is the type for traffic isolation configuration
type TrafficIsolationType string

const (
	// ConfigName is the name of config map for Kourier.
	ConfigName = "config-kourier"

	// enableServiceAccessLoggingKey is the config map key for enabling service related
	// access logging.
	enableServiceAccessLoggingKey = "enable-service-access-logging"

	// enableProxyProtocol is the config map key for enabling proxy protocol
	enableProxyProtocol = "enable-proxy-protocol"

	// clusterCert is the config map key for kourier internal certificates
	clusterCert = "cluster-cert-secret"

	// IdleTimeoutKey is the config map key for the amount of time that Kourier waits
	// for incoming requests. This value is set to "stream_idle_timeout" in Envoy.
	IdleTimeoutKey = "stream-idle-timeout"

	// trafficIsolation is the config map key for controlling the desire level of incoming traffic isolation
	trafficIsolation = "traffic-isolation"

	// IsolationIngressPort if the config map value enabling port-level traffic isolation
	IsolationIngressPort TrafficIsolationType = "port"
)

func DefaultConfig() *Kourier {
	return &Kourier{
		EnableServiceAccessLogging: true, // true is the default for backwards-compat
		EnableProxyProtocol:        false,
		ClusterCertSecret:          "",
		IdleTimeout:                0 * time.Second, // default value
		TrafficIsolation:           "",
	}
}

// NewConfigFromMap creates a DeploymentConfig from the supplied Map.
func NewConfigFromMap(configMap map[string]string) (*Kourier, error) {
	nc := DefaultConfig()

	if err := cm.Parse(configMap,
		cm.AsBool(enableServiceAccessLoggingKey, &nc.EnableServiceAccessLogging),
		cm.AsBool(enableProxyProtocol, &nc.EnableProxyProtocol),
		cm.AsString(clusterCert, &nc.ClusterCertSecret),
		cm.AsDuration(IdleTimeoutKey, &nc.IdleTimeout),
		cm.AsString(trafficIsolation, (*string)(&nc.TrafficIsolation)),
	); err != nil {
		return nil, err
	}

	return nc, nil
}

// NewConfigFromConfigMap creates a Kourier from the supplied configMap.
func NewConfigFromConfigMap(config *corev1.ConfigMap) (*Kourier, error) {
	return NewConfigFromMap(config.Data)
}

// Kourier includes the configuration for Kourier.
// +k8s:deepcopy-gen=true
type Kourier struct {
	// EnableServiceAccessLogging specifies whether requests reaching the Kourier gateway
	// should be logged.
	EnableServiceAccessLogging bool
	// EnableProxyProtocol specifies whether proxy protocol feature is enabled
	EnableProxyProtocol bool
	// ClusterCertSecret specifies the secret name for the server certificates of
	// Kourier Internal.
	ClusterCertSecret string
	// IdleTimeout specifies the amount of time that Kourier waits for incoming requests.
	// The default value is 5 minutes. This will not interfere any smaller configured
	// timeouts that may have existed in configurations prior to
	// this option, for example, the "timeoutSeconds" specified in Knative service is still
	// valid.
	IdleTimeout time.Duration
	// Desire level of incoming traffic isolation
	TrafficIsolation TrafficIsolationType
}
