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
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/kelseyhightower/envconfig"
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

	// serviceAccessLogTemplateKey is the config map key for the access log template.
	serviceAccessLogTemplateKey = "service-access-log-template"

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

	disableEnvoyServerHeader = "disable-envoy-server-header"

	extauthzHostKey                = "extauthz-host"
	extauthzProtocolKey            = "extauthz-protocol"
	extauthzFailureModeAllowKey    = "extauthz-failure-mode-allow"
	extauthzMaxRequestBodyBytesKey = "extauthz-max-request-body-bytes"
	extauthzTimeoutKey             = "extauthz-timeout"
	extauthzPathPrefixKey          = "extauthz-path-prefix"
	extauthzPackAsBytesKey         = "extauthz-pack-as-bytes"
)

func defaultKourierConfig() *Kourier {
	return &Kourier{
		EnableServiceAccessLogging: true, // true is the default for backwards-compat
		ServiceAccessLogTemplate:   "",
		EnableProxyProtocol:        false,
		ClusterCertSecret:          "",
		IdleTimeout:                0 * time.Second, // default value
		TrustedHopsCount:           0,
		CipherSuites:               nil,
		EnableCryptoMB:             false,
		UseRemoteAddress:           false,
		DisableEnvoyServerHeader:   false,
		ExternalAuthz: ExternalAuthz{
			Enabled: false,
		},
	}
}

// NewKourierConfigFromMap creates a KourierConfig from the supplied Map.
func NewKourierConfigFromMap(configMap map[string]string) (*Kourier, error) {
	nc := defaultKourierConfig()

	if err := cm.Parse(configMap,
		cm.AsBool(enableServiceAccessLoggingKey, &nc.EnableServiceAccessLogging),
		cm.AsString(serviceAccessLogTemplateKey, &nc.ServiceAccessLogTemplate),
		cm.AsBool(enableProxyProtocol, &nc.EnableProxyProtocol),
		cm.AsString(clusterCert, &nc.ClusterCertSecret),
		cm.AsDuration(IdleTimeoutKey, &nc.IdleTimeout),
		cm.AsUint32(trustedHopsCount, &nc.TrustedHopsCount),
		cm.AsBool(useRemoteAddress, &nc.UseRemoteAddress),
		cm.AsStringSet(cipherSuites, &nc.CipherSuites),
		cm.AsBool(enableCryptoMB, &nc.EnableCryptoMB),
		asTracing(TracingCollectorFullEndpoint, &nc.Tracing),
		asExternalAuthz(&nc.ExternalAuthz),
		cm.AsBool(disableEnvoyServerHeader, &nc.DisableEnvoyServerHeader),
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

func asExternalAuthz(externalAuthz *ExternalAuthz) cm.ParseFunc {
	return func(data map[string]string) error {
		config := defaultExternalAuthzConfig()
		var host string

		// For backward compatibility, if KOURIER_EXTAUTHZ_HOST is set, use it.
		if host = os.Getenv("KOURIER_EXTAUTHZ_HOST"); host != "" {
			if err := envconfig.Process("KOURIER_EXTAUTHZ", &config); err != nil {
				return fmt.Errorf("failed to parse external authz config: %w", err)
			}
		} else {
			host = data[extauthzHostKey]
			if host == "" {
				return nil
			}

			protocol := extAuthzProtocol(data[extauthzProtocolKey])
			if !isValidExtAuthzProtocol(protocol) {
				return fmt.Errorf("protocol %s is invalid, must be in %+v", protocol, extAuthzProtocols)
			}
			config.Protocol = protocol

			if err := cm.Parse(data,
				cm.AsBool(extauthzFailureModeAllowKey, &config.FailureModeAllow),
				cm.AsUint32(extauthzMaxRequestBodyBytesKey, &config.MaxRequestBytes),
				cm.AsInt(extauthzTimeoutKey, &config.Timeout),
				cm.AsString(extauthzPathPrefixKey, &config.PathPrefix),
				cm.AsBool(extauthzPackAsBytesKey, &config.PackAsBytes),
			); err != nil {
				return fmt.Errorf("failed to parse external authz config: %w", err)
			}
		}

		h, portStr, err := net.SplitHostPort(host)
		if err != nil {
			return fmt.Errorf("failed to split host and port from %s: %w", host, err)
		}

		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("failed to convert port %s to int: %w", portStr, err)
		}

		if port > unixMaxPort {
			// Bail out if we exceed the maximum port number.
			return fmt.Errorf("port %d bigger than %d", port, unixMaxPort)
		}

		// When using environments to get a host with port,
		// it should be overwritten by a host without port.
		config.Host = h
		//nolint:gosec // port is below unixMaxPort
		config.Port = uint32(port)

		externalAuthz.Enabled = true
		externalAuthz.Config = config

		return nil
	}
}

// NewConfigFromConfigMap creates a Kourier from the supplied configMap.
func NewKourierConfigFromConfigMap(config *corev1.ConfigMap) (*Kourier, error) {
	return NewKourierConfigFromMap(config.Data)
}

// Kourier includes the configuration for Kourier.
// +k8s:deepcopy-gen=true
type Kourier struct {
	// EnableServiceAccessLogging specifies whether requests reaching the Kourier gateway
	// should be logged.
	EnableServiceAccessLogging bool
	// ServiceAccessLogTemplate specifies the format of the access log used by the Kourier gateway.
	// This template follows the envoy format.
	// see: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage#access-logging
	ServiceAccessLogTemplate string
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
	// UseRemoteAddress configures the connection manager to use the real remote address
	// of the client connection when determining internal versus external origin and manipulating various headers.
	UseRemoteAddress bool
	// EnableCryptoMB specifies whether Kourier enable CryptoMB private provider to accelerate
	// TLS handshake. The default value is "false".
	EnableCryptoMB bool
	// CipherSuites specifies the cipher suites for TLS external listener.
	CipherSuites sets.Set[string]
	// Tracing specifies the configuration for gateway tracing
	Tracing Tracing
	// Disable Server Header
	DisableEnvoyServerHeader bool
	// ExternalAuthz is the configuration for external authorization.
	ExternalAuthz ExternalAuthz
}
