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
	corev1 "k8s.io/api/core/v1"

	cm "knative.dev/pkg/configmap"
)

const (
	// ConfigName is the name of config map for Kourier.
	ConfigName = "config-kourier"

	// enableServiceAccessLoggingKey is the config map key for enabling service related
	// access logging.
	enableServiceAccessLoggingKey = "enable-service-access-logging"

	// enableProxyProtocol is the config map key for enabling proxy protocol
	enableProxyProtocol = "enable-proxy-protocol"
)

func DefaultConfig() *Kourier {
	return &Kourier{
		EnableServiceAccessLogging: true,  // true is the default for backwards-compat
		EnableProxyProtocol:        false,
	}
}

// NewConfigFromMap creates a DeploymentConfig from the supplied Map.
func NewConfigFromMap(configMap map[string]string) (*Kourier, error) {
	nc := DefaultConfig()

	if err := cm.Parse(configMap,
		cm.AsBool(enableServiceAccessLoggingKey, &nc.EnableServiceAccessLogging),
		cm.AsBool(enableProxyProtocol, &nc.EnableProxyProtocol),
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
}
