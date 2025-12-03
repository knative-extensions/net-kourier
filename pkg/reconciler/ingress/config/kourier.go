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
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	cm "knative.dev/pkg/configmap"
	"knative.dev/pkg/observability/metrics"
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

	// listenIPAddressesKey receives the list of IP addresses to listen to.
	listenIPAddressesKey = "listen-ip-addresses"

	TracingEndpointKey     = "tracing-endpoint"
	TracingProtocolKey     = "tracing-protocol"
	TracingSamplingRateKey = "tracing-sampling-rate"
	TracingServiceNameKey  = "tracing-service-name"

	disableEnvoyServerHeader = "disable-envoy-server-header"

	extauthzHostKey                = "extauthz-host"
	extauthzProtocolKey            = "extauthz-protocol"
	extauthzFailureModeAllowKey    = "extauthz-failure-mode-allow"
	extauthzMaxRequestBodyBytesKey = "extauthz-max-request-body-bytes"
	extauthzTimeoutKey             = "extauthz-timeout"
	extauthzPathPrefixKey          = "extauthz-path-prefix"
	extauthzPackAsBytesKey         = "extauthz-pack-as-bytes"

	certsSecretNameKey      = "certs-secret-name"
	certsSecretNamespaceKey = "certs-secret-namespace"

	EnvCertsSecretName      = "CERTS_SECRET_NAME"
	EnvCertsSecretNamespace = "CERTS_SECRET_NAMESPACE"
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
		ListenIPAddresses:          []string{"0.0.0.0"},
		UseRemoteAddress:           false,
		DisableEnvoyServerHeader:   false,
		ExternalAuthz: ExternalAuthz{
			Enabled: false,
		},
		// For backward compatibility, if CERTS_SECRET_NAME and CERTS_SECRET_NAMESPACE is set, use it.
		CertsSecretName:      os.Getenv(EnvCertsSecretName),
		CertsSecretNamespace: os.Getenv(EnvCertsSecretNamespace),
	}
}

// NewKourierConfigFromMap creates a KourierConfig from the supplied Map.
func NewKourierConfigFromMap(configMap map[string]string) (*Kourier, error) {
	nc := defaultKourierConfig()

	if err := cm.Parse(configMap,
		cm.AsBool(enableServiceAccessLoggingKey, &nc.EnableServiceAccessLogging),
		// Avoid using cm.AsString, as it also removes newline characters at the end.
		asServiceAccessLogTemplate(serviceAccessLogTemplateKey, &nc.ServiceAccessLogTemplate),
		cm.AsBool(enableProxyProtocol, &nc.EnableProxyProtocol),
		cm.AsString(clusterCert, &nc.ClusterCertSecret),
		cm.AsDuration(IdleTimeoutKey, &nc.IdleTimeout),
		cm.AsUint32(trustedHopsCount, &nc.TrustedHopsCount),
		cm.AsBool(useRemoteAddress, &nc.UseRemoteAddress),
		cm.AsStringSet(cipherSuites, &nc.CipherSuites),
		cm.AsBool(enableCryptoMB, &nc.EnableCryptoMB),
		asListenIPAddresses(&nc.ListenIPAddresses),
		asTracing(&nc.Tracing),
		asExternalAuthz(&nc.ExternalAuthz),
		cm.AsBool(disableEnvoyServerHeader, &nc.DisableEnvoyServerHeader),
		cm.AsString(certsSecretNameKey, &nc.CertsSecretName),
		cm.AsString(certsSecretNamespaceKey, &nc.CertsSecretNamespace),
	); err != nil {
		return nil, err
	}

	return nc, nil
}

func asServiceAccessLogTemplate(key string, v *string) cm.ParseFunc {
	return func(data map[string]string) error {
		if raw, ok := data[key]; ok {
			*v = raw
		}
		return nil
	}
}

func asTracing(tracing *Tracing) cm.ParseFunc {
	return func(data map[string]string) error {
		otlpEndpoint := data[TracingEndpointKey]
		if otlpEndpoint == "" {
			return nil
		}

		tracing.Enabled = true
		tracing.Endpoint = otlpEndpoint

		// Parse endpoint to extract host, port, and path
		host, port, path, err := parseOtlpEndpoint(otlpEndpoint)
		if err != nil {
			return fmt.Errorf("invalid tracing endpoint %q: %w", otlpEndpoint, err)
		}
		tracing.OTLPHost = host
		tracing.OTLPPort = port
		tracing.OTLPPath = path

		// Parse protocol
		protocol := data[TracingProtocolKey]
		if protocol == "" {
			tracing.Protocol = DefaultTracingProtocol
		} else if protocol != metrics.ProtocolHTTPProtobuf && protocol != metrics.ProtocolGRPC {
			return fmt.Errorf("invalid tracing protocol %q, must be %q or %q", protocol, metrics.ProtocolHTTPProtobuf, metrics.ProtocolGRPC)
		} else {
			tracing.Protocol = protocol
		}

		// Parse sampling rate
		samplingRateStr := data[TracingSamplingRateKey]
		if samplingRateStr == "" {
			tracing.SamplingRate = DefaultTracingSamplingRate
		} else {
			samplingRate, err := strconv.ParseFloat(samplingRateStr, 64)
			if err != nil {
				return fmt.Errorf("invalid tracing sampling rate %q: %w", samplingRateStr, err)
			}
			if samplingRate < 0.0 || samplingRate > 1.0 {
				return fmt.Errorf("tracing sampling rate %f must be between 0.0 and 1.0", samplingRate)
			}
			tracing.SamplingRate = samplingRate
		}

		// Parse service name
		serviceName := data[TracingServiceNameKey]
		if serviceName == "" {
			tracing.ServiceName = DefaultTracingServiceName
		} else {
			tracing.ServiceName = serviceName
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

// NewKourierConfigFromConfigMap creates a Kourier from the supplied configMap.
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
	// The main IP address to listen to
	ListenIPAddresses []string
	// CipherSuites specifies the cipher suites for TLS external listener.
	CipherSuites sets.Set[string]
	// Tracing specifies the configuration for gateway tracing
	Tracing Tracing
	// Disable Server Header
	DisableEnvoyServerHeader bool
	// ExternalAuthz is the configuration for external authorization.
	ExternalAuthz ExternalAuthz
	// CertsSecretName is the name of the secret containing the TLS certificates for the Kourier gateway.
	CertsSecretName string
	// CertsSecretNamespace is the namespace of the secret containing the TLS certificates for the Kourier gateway.
	CertsSecretNamespace string
}

// UseHTTPSListenerWithOneCert returns true if we need to modify the HTTPS listener with just one cert
// instead of one per ingress.
func (k *Kourier) UseHTTPSListenerWithOneCert() bool {
	return k.CertsSecretName != "" && k.CertsSecretNamespace != ""
}

// asListenIPAddresses parses and validates a comma-separated list of IP addresses.
func asListenIPAddresses(target *[]string) cm.ParseFunc {
	return func(data map[string]string) error {
		raw, ok := data[listenIPAddressesKey]
		if !ok {
			return nil
		}

		ips := strings.Split(raw, ",")
		for i, v := range ips {
			ips[i] = strings.TrimSpace(v)
		}

		if len(ips) == 0 {
			return fmt.Errorf("%s must contain at least one IP address", listenIPAddressesKey)
		}

		for _, ip := range ips {
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("invalid IP address %q in %s", ip, listenIPAddressesKey)
			}
		}

		*target = ips
		return nil
	}
}
