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
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

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

	// clusterCert is the config map key for kourier internal certificates
	clusterCert = "cluster-cert-secret"

	// IdleTimeoutKey is the config map key for the amount of time that Kourier waits
	// for incoming requests. This value is set to "stream_idle_timeout" in Envoy.
	IdleTimeoutKey = "stream-idle-timeout"

	// enableCryptoMB is the config map for enabling CryptoMB private key provider.
	enableCryptoMB = "enable-cryptomb"

	// TracingCollectorFullEndpoint is the config map key to configure tracing at kourier gateway level
	TracingCollectorFullEndpoint = "tracing-collector-full-endpoint"
)

func DefaultConfig() *Kourier {
	return &Kourier{
		EnableServiceAccessLogging: true, // true is the default for backwards-compat
		EnableProxyProtocol:        false,
		ClusterCertSecret:          "",
		IdleTimeout:                0 * time.Second, // default value
		TrustedHopsCount:           0,
		CipherSuites:               nil,
		EnableCryptoMB:             false,
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
		cm.AsUint32(trustedHopsCount, &nc.TrustedHopsCount),
		cm.AsStringSet(cipherSuites, &nc.CipherSuites),
		cm.AsBool(enableCryptoMB, &nc.EnableCryptoMB),
		asTracing(TracingCollectorFullEndpoint, &nc.Tracing),
	); err != nil {
		return nil, err
	}

	return nc, nil
}

// Tracing contains all fields required to configure tracing at kourier gateway level.
// This object is mostly filled by the asTracing method, using TracingCollectorFullEndpoint value as the source.
type Tracing struct {
	Enabled           bool
	CollectorHost     string
	CollectorPort     uint16
	CollectorEndpoint string
}

func asTracing(collectorFullEndpoint string, tracing *Tracing) cm.ParseFunc {
	return func(data map[string]string) error {
		if raw, ok := data[collectorFullEndpoint]; ok && raw != "" {
			tracing.Enabled = true

			// We add a random scheme to be able to use url.ParseRequestURI.
			parsedURL, err := url.ParseRequestURI("scheme://" + raw)
			if err != nil {
				return fmt.Errorf("%q is not a valid URL: %w", raw, err)
			}

			tracing.CollectorHost = parsedURL.Hostname()
			collectorPortUint64, err := strconv.ParseUint(parsedURL.Port(), 10, 32)
			if err != nil {
				return fmt.Errorf("%q is not a valid port: %w", parsedURL.Port(), err)
			}

			if collectorPortUint64 > math.MaxUint16 {
				return fmt.Errorf("port %d must be a valid port", collectorPortUint64)
			}

			tracing.CollectorPort = uint16(collectorPortUint64)
			tracing.CollectorEndpoint = parsedURL.Path
		}

		return nil
	}
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
	// TrustedHopsCount configures the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust.
	TrustedHopsCount uint32
	// EnableCryptoMB specifies whether Kourier enable CryptoMB private provider to accelerate
	// TLS handshake. The default value is "false".
	EnableCryptoMB bool
	// CipherSuites specifies the cipher suites for TLS external listener.
	CipherSuites sets.Set[string]
	// Tracing specifies the configuration for gateway tracing
	Tracing Tracing
}
