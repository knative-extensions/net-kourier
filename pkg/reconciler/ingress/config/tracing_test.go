/*
Copyright 2025 The Knative Authors

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

	envoy_config_trace_v3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	httpOptions "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"knative.dev/pkg/observability/metrics"
)

func TestTracingCluster_GrpcProtocolEnablesHTTP2(t *testing.T) {
	tracing := Tracing{
		Enabled:  true,
		Protocol: metrics.ProtocolGRPC,
		OTLPHost: "otel-collector",
		OTLPPort: 4317,
	}

	cluster := tracing.Cluster()
	if cluster == nil {
		t.Fatal("expected cluster to be non-nil for enabled tracing")
	}

	// gRPC requires HTTP/2 to be configured on the cluster
	h2OptionsAny, ok := cluster.TypedExtensionProtocolOptions["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	if !ok {
		t.Fatal("gRPC protocol must configure HTTP/2 protocol options")
	}

	var h2Options httpOptions.HttpProtocolOptions
	if err := h2OptionsAny.UnmarshalTo(&h2Options); err != nil {
		t.Fatalf("failed to unmarshal HTTP protocol options: %v", err)
	}

	if h2Options.GetExplicitHttpConfig().GetHttp2ProtocolOptions() == nil {
		t.Error("expected HTTP/2 protocol options to be configured for gRPC")
	}
}

func TestTracingCluster_HttpProtocolDoesNotEnableHTTP2(t *testing.T) {
	tracing := Tracing{
		Enabled:  true,
		Protocol: metrics.ProtocolHTTPProtobuf,
		OTLPHost: "otel-collector",
		OTLPPort: 4318,
	}

	cluster := tracing.Cluster()
	if cluster == nil {
		t.Fatal("expected cluster to be non-nil for enabled tracing")
	}

	// HTTP protocol should not configure HTTP/2
	if len(cluster.TypedExtensionProtocolOptions) != 0 {
		t.Error("HTTP protocol should not configure HTTP/2 protocol options")
	}
}

func TestTracingCluster_DisabledReturnsNil(t *testing.T) {
	tracing := Tracing{
		Enabled:  false,
		Protocol: metrics.ProtocolHTTPProtobuf,
	}

	if tracing.Cluster() != nil {
		t.Error("expected nil cluster for disabled tracing")
	}
}

func TestTracingConfig_HttpProtocolUsesHttpService(t *testing.T) {
	tracing := Tracing{
		Enabled:      true,
		Protocol:     metrics.ProtocolHTTPProtobuf,
		SamplingRate: 0.67,
		ServiceName:  "test-service",
	}

	config := tracing.TracingConfig()
	if config == nil {
		t.Fatal("expected config to be non-nil for enabled tracing")
	}

	var otelConfig envoy_config_trace_v3.OpenTelemetryConfig
	if err := config.Provider.GetTypedConfig().UnmarshalTo(&otelConfig); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	// HTTP protocol uses HttpService with 5s timeout
	if otelConfig.HttpService == nil {
		t.Fatal("HTTP protocol must use HttpService")
	}
	if got := otelConfig.HttpService.HttpUri.Timeout; got.AsDuration() != 5*time.Second {
		t.Errorf("HTTP timeout = %v, want 5s", got.AsDuration())
	}
	if otelConfig.ServiceName != "test-service" {
		t.Errorf("ServiceName = %q, want %q", otelConfig.ServiceName, "test-service")
	}
}

func TestTracingConfig_GrpcProtocolUsesGrpcService(t *testing.T) {
	tracing := Tracing{
		Enabled:      true,
		Protocol:     metrics.ProtocolGRPC,
		SamplingRate: 1.0,
		ServiceName:  "test-service",
	}

	config := tracing.TracingConfig()
	if config == nil {
		t.Fatal("expected config to be non-nil for enabled tracing")
	}

	var otelConfig envoy_config_trace_v3.OpenTelemetryConfig
	if err := config.Provider.GetTypedConfig().UnmarshalTo(&otelConfig); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	// gRPC protocol uses GrpcService with 250ms timeout
	if otelConfig.GrpcService == nil {
		t.Fatal("gRPC protocol must use GrpcService")
	}
	if got := otelConfig.GrpcService.Timeout; got.AsDuration() != 250*time.Millisecond {
		t.Errorf("gRPC timeout = %v, want 250ms", got.AsDuration())
	}
	if otelConfig.ServiceName != "test-service" {
		t.Errorf("ServiceName = %q, want %q", otelConfig.ServiceName, "test-service")
	}
}

func TestTracingConfig_SamplingRateConversion(t *testing.T) {
	tracing := Tracing{
		Enabled:      true,
		Protocol:     metrics.ProtocolHTTPProtobuf,
		SamplingRate: 0.67,
	}

	config := tracing.TracingConfig()
	if config == nil {
		t.Fatal("expected config to be non-nil for enabled tracing")
	}

	// Sampling rate should be converted from 0-1 range to 0-100 percentage
	if got := config.OverallSampling.Value; got != 67.0 {
		t.Errorf("OverallSampling.Value = %v, want 67.0", got)
	}
}

func TestTracingConfig_DisabledReturnsNil(t *testing.T) {
	tracing := Tracing{
		Enabled:  false,
		Protocol: metrics.ProtocolHTTPProtobuf,
	}

	if tracing.TracingConfig() != nil {
		t.Error("expected nil config for disabled tracing")
	}
}

func TestParseOtlpEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		wantHost string
		wantPort uint32
		wantPath string
		wantErr  bool
	}{
		{"http://otel-collector:4318/v1/traces", "otel-collector", 4318, "/v1/traces", false},
		{"http://otel-collector:4317", "otel-collector", 4317, "", false},
		{"http://[::1]:4318/v1/traces", "::1", 4318, "/v1/traces", false},
		{"http://otel-collector/v1/traces", "", 0, "", true}, // missing port
		{"not a url", "", 0, "", true},                       // invalid URL
	}

	for _, tt := range tests {
		host, port, path, err := parseOtlpEndpoint(tt.endpoint)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseOtlpEndpoint(%q) expected error, got nil", tt.endpoint)
			}
		} else {
			if err != nil {
				t.Errorf("parseOtlpEndpoint(%q) unexpected error: %v", tt.endpoint, err)
				continue
			}
			if host != tt.wantHost {
				t.Errorf("parseOtlpEndpoint(%q) host = %q, want %q", tt.endpoint, host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("parseOtlpEndpoint(%q) port = %d, want %d", tt.endpoint, port, tt.wantPort)
			}
			if path != tt.wantPath {
				t.Errorf("parseOtlpEndpoint(%q) path = %q, want %q", tt.endpoint, path, tt.wantPath)
			}
		}
	}
}
