//go:build e2e
// +build e2e

/*
Copyright 2023 The Knative Authors

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

package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	jaegerAPI "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/discovery"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
	"knative.dev/networking/test/types"
	"knative.dev/pkg/observability/metrics"
)

const (
	configMapName      = "config-kourier"
	jaegerNamespace    = "tracing"
	jaegerServiceName  = "jaeger"
	kourierNamespace   = "knative-serving"
	testsNamespace     = "serving-tests"
	tracingServiceName = "kourier-test"

	configReloadTimeout = 10 * time.Second
	traceWaitTime       = 12 * time.Second
)

// TestTracingProtocols verifies that both HTTP and gRPC tracing protocols work correctly
func TestTracingProtocols(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

	tests := []struct {
		protocol string
	}{
		{protocol: metrics.ProtocolGRPC},
		{protocol: metrics.ProtocolHTTPProtobuf},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			setupTracingConfig(t, ctx, clients, tt.protocol)
			startTime := time.Now()
			name, _, client, ing, url := createTestIngressAndService(ctx, t, clients)

			makeRequest(t, client, url)

			t.Logf("Waiting %v for traces to be collected and exported", traceWaitTime)
			time.Sleep(traceWaitTime)
			numberOfTracesFromHTTPCalls := queryJaegerTraces(ctx, t, clients,
				map[string]string{
					"http.status_code": "200",
					"http.url":         url + "/",
					"upstream_cluster": testsNamespace + "/" + name,
					"user_agent":       fmt.Sprintf("knative.dev/%s/%s", t.Name(), ing.Name),
				},
				startTime,
			)
			if numberOfTracesFromHTTPCalls != 1 {
				t.Errorf("expected exactly 1 trace from HTTP call, got %d", numberOfTracesFromHTTPCalls)
			}

			numberOfTracesFromKnativeIngressProbe := queryJaegerTraces(ctx, t, clients,
				map[string]string{
					"http.status_code": "200",
					"http.url":         url + "/healthz",
					"upstream_cluster": testsNamespace + "/" + name,
					"user_agent":       header.IngressReadinessUserAgent,
				},
				startTime,
			)
			if numberOfTracesFromKnativeIngressProbe < 1 {
				t.Errorf("expected at least 1 trace from Knative probe, got %d", numberOfTracesFromKnativeIngressProbe)
			}

			kubeVersion, err := discovery.NewDiscoveryClient(clients.KubeClient.DiscoveryV1().RESTClient()).ServerVersion()
			if err != nil {
				t.Fatal(err)
			}

			numberOfTracesFromKubeProbe := queryJaegerTraces(ctx, t, clients,
				map[string]string{
					"http.status_code": "200",
					"http.url":         "http://" + config.InternalKourierDomain + "/ready",
					"upstream_cluster": generator.ServiceStatsClusterName,
					"user_agent":       fmt.Sprintf("%s%s.%s", header.KubeProbeUAPrefix, kubeVersion.Major, kubeVersion.Minor),
				},
				startTime,
			)
			if numberOfTracesFromKubeProbe < 1 {
				t.Errorf("expected at least 1 trace from Kube probe, got %d", numberOfTracesFromKubeProbe)
			}

			t.Logf("Protocol %s: Successfully verified %d HTTP call traces, %d Knative probe traces, %d Kube probe traces",
				tt.protocol, numberOfTracesFromHTTPCalls, numberOfTracesFromKnativeIngressProbe, numberOfTracesFromKubeProbe)
		})
	}
}

// TestTracingDisabled verifies that no traces are collected when tracing is disabled
func TestTracingDisabled(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

	setupTracingConfig(t, ctx, clients, "")
	startTime := time.Now()
	name, _, client, _, url := createTestIngressAndService(ctx, t, clients)

	makeRequest(t, client, url)

	t.Logf("Waiting %v for potential traces", traceWaitTime)
	time.Sleep(traceWaitTime)
	numberOfTraces := queryJaegerTraces(ctx, t, clients,
		map[string]string{
			"http.status_code": "200",
			"upstream_cluster": testsNamespace + "/" + name,
		},
		startTime,
	)

	if numberOfTraces != 0 {
		t.Errorf("expected 0 traces when tracing is disabled, got %d", numberOfTraces)
	}
}

// TestTracingSamplingRate verifies that sampling rate configuration is respected
func TestTracingSamplingRate(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

	tests := []struct {
		name             string
		samplingRate     string
		numRequests      int
		expectedTraces   int
		tolerancePercent int // acceptable tolerance as percentage (0-100), 0 = exact match
	}{
		{
			name:           "sampling rate 0.0 (no traces)",
			samplingRate:   "0.0",
			numRequests:    10,
			expectedTraces: 0,
		},
		{
			name:             "sampling rate 0.67 (partial traces)",
			samplingRate:     "0.67",
			numRequests:      30,
			expectedTraces:   20,
			tolerancePercent: 20,
		},
		{
			name:           "sampling rate 1.0 (all traces)",
			samplingRate:   "1.0",
			numRequests:    10,
			expectedTraces: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTracingConfigWithSamplingRate(t, ctx, clients, "grpc", tt.samplingRate)

			startTime := time.Now()
			name, _, client, _, url := createTestIngressAndService(ctx, t, clients)

			for range tt.numRequests {
				makeRequest(t, client, url)
			}

			t.Logf("Waiting %v for traces", traceWaitTime)
			time.Sleep(traceWaitTime)
			numberOfTraces := queryJaegerTraces(ctx, t, clients,
				map[string]string{
					"http.status_code": "200",
					"upstream_cluster": testsNamespace + "/" + name,
					"http.url":         url + "/", // Filter for root path, excluding /healthz and /ready probes
				},
				startTime,
			)

			if tt.tolerancePercent == 0 {
				// Exact match required
				t.Logf("Sampling rate %s: got %d traces out of %d requests (expected exactly %d)",
					tt.samplingRate, numberOfTraces, tt.numRequests, tt.expectedTraces)
				if numberOfTraces != tt.expectedTraces {
					t.Errorf("got %d traces, expected exactly %d", numberOfTraces, tt.expectedTraces)
				}
			} else {
				// Range check with tolerance
				delta := (tt.expectedTraces * tt.tolerancePercent) / 100
				minExpected := tt.expectedTraces - delta
				maxExpected := tt.expectedTraces + delta
				t.Logf("Sampling rate %s: got %d traces out of %d requests (expected %d ±%d%%)",
					tt.samplingRate, numberOfTraces, tt.numRequests, tt.expectedTraces, tt.tolerancePercent)

				if numberOfTraces < minExpected {
					t.Errorf("got %d traces, expected at least %d (target %d ±%d%%)",
						numberOfTraces, minExpected, tt.expectedTraces, tt.tolerancePercent)
				}
				if numberOfTraces > maxExpected {
					t.Errorf("got %d traces, expected at most %d (target %d ±%d%%)",
						numberOfTraces, maxExpected, tt.expectedTraces, tt.tolerancePercent)
				}
			}
		})
	}
}

// TestTracingW3CContextPropagation verifies that W3C trace context headers are preserved and propagated
func TestTracingW3CContextPropagation(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

	setupTracingConfig(t, ctx, clients, "grpc")
	startTime := time.Now()
	name, _, client, _, url := createTestIngressAndService(ctx, t, clients)

	// W3C traceparent format: version-trace-id-parent-id-trace-flags
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	traceparent := "00-" + traceID + "-00f067aa0ba902b7-01"
	tracestate := "vendor1=value1,vendor2=value2"
	baggage := "key1=value1,key2=value2"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Traceparent", traceparent)
	req.Header.Set("Tracestate", tracestate)
	req.Header.Set("Baggage", baggage)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var runtimeInfo types.RuntimeInfo
	if err := json.NewDecoder(resp.Body).Decode(&runtimeInfo); err != nil {
		t.Fatal(err)
	}

	// Verify W3C trace context headers were propagated to the backend service
	gotTraceparent := runtimeInfo.Request.Headers.Get("Traceparent")
	// Verify trace-id is preserved (parent-id will be different for the new span)
	parts := strings.SplitN(gotTraceparent, "-", 4)
	if parts[1] != traceID {
		t.Errorf("traceparent trace-id not preserved: got %q, want %q", parts[1], traceID)
	}

	if got := runtimeInfo.Request.Headers.Get("Tracestate"); got != tracestate {
		t.Errorf("tracestate not propagated to backend: got %q, want %q", got, tracestate)
	}

	if got := runtimeInfo.Request.Headers.Get("Baggage"); got != baggage {
		t.Errorf("baggage not propagated to backend: got %q, want %q", got, baggage)
	}

	t.Logf("Waiting %v for traces to be collected and exported", traceWaitTime)
	time.Sleep(traceWaitTime)

	// Query for traces that should be linked to our trace ID
	numberOfTraces := queryJaegerTraces(ctx, t, clients,
		map[string]string{
			"http.status_code": "200",
			"http.url":         url + "/",
			"upstream_cluster": testsNamespace + "/" + name,
		},
		startTime,
	)

	if numberOfTraces < 1 {
		t.Errorf("expected at least 1 trace with W3C context propagation, got %d", numberOfTraces)
	}

	t.Logf("Successfully verified trace propagation with W3C context headers")
}

func setupTracingConfig(t *testing.T, ctx context.Context, clients *test.Clients, protocol string) {
	t.Helper()
	setupTracingConfigWithSamplingRate(t, ctx, clients, protocol, "1.0")
}

func setupTracingConfigWithSamplingRate(t *testing.T, ctx context.Context, clients *test.Clients, protocol, samplingRate string) {
	t.Helper()

	cm, err := clients.KubeClient.CoreV1().ConfigMaps(kourierNamespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	originalEndpoint := cm.Data[config.TracingEndpointKey]
	originalProtocol := cm.Data[config.TracingProtocolKey]
	originalSamplingRate := cm.Data[config.TracingSamplingRateKey]
	originalServiceName := cm.Data[config.TracingServiceNameKey]

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	endpoint := getJaegerEndpoint(protocol)
	if endpoint == "" {
		delete(cm.Data, config.TracingEndpointKey)
		delete(cm.Data, config.TracingProtocolKey)
		delete(cm.Data, config.TracingSamplingRateKey)
		delete(cm.Data, config.TracingServiceNameKey)
	} else {
		cm.Data[config.TracingEndpointKey] = endpoint
		cm.Data[config.TracingProtocolKey] = protocol
		cm.Data[config.TracingSamplingRateKey] = samplingRate
		cm.Data[config.TracingServiceNameKey] = tracingServiceName
	}

	_, err = clients.KubeClient.CoreV1().ConfigMaps(kourierNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Updated tracing config: protocol=%s, endpoint=%s, samplingRate=%s, serviceName=%s", protocol, endpoint, samplingRate, tracingServiceName)

	t.Cleanup(func() {
		cm, err := clients.KubeClient.CoreV1().ConfigMaps(kourierNamespace).Get(ctx, configMapName, metav1.GetOptions{})
		if err != nil {
			t.Logf("Warning: Failed to get ConfigMap for cleanup: %v", err)
			return
		}

		if originalEndpoint == "" {
			delete(cm.Data, config.TracingEndpointKey)
			delete(cm.Data, config.TracingProtocolKey)
			delete(cm.Data, config.TracingSamplingRateKey)
			delete(cm.Data, config.TracingServiceNameKey)
		} else {
			cm.Data[config.TracingEndpointKey] = originalEndpoint
			cm.Data[config.TracingProtocolKey] = originalProtocol
			cm.Data[config.TracingSamplingRateKey] = originalSamplingRate
			cm.Data[config.TracingServiceNameKey] = originalServiceName
		}

		_, err = clients.KubeClient.CoreV1().ConfigMaps(kourierNamespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			t.Logf("Warning: Failed to restore ConfigMap: %v", err)
		}
	})

	time.Sleep(configReloadTimeout)
}

func createTestIngressAndService(ctx context.Context, t *testing.T, clients *test.Clients) (name string, port int, client *http.Client, ing *v1alpha1.Ingress, url string) {
	t.Helper()

	name, port, _ = ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	host := name + "." + test.NetworkingFlags.ServiceDomain
	url = "http://" + host

	ing, client, _ = ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts:      []string{host},
			Visibility: v1alpha1.IngressVisibilityExternalIP,
			HTTP: &v1alpha1.HTTPIngressRuleValue{
				Paths: []v1alpha1.HTTPIngressPath{{
					Splits: []v1alpha1.IngressBackendSplit{{
						IngressBackend: v1alpha1.IngressBackend{
							ServiceName:      name,
							ServiceNamespace: test.ServingNamespace,
							ServicePort:      intstr.FromInt(port),
						},
					}},
				}},
			},
		}},
	})

	return name, port, client, ing, url
}

func makeRequest(t *testing.T, client *http.Client, url string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

func queryJaegerTraces(ctx context.Context, t *testing.T, clients *test.Clients, tags map[string]string, startTime time.Time) int {
	t.Helper()

	svc, err := clients.KubeClient.CoreV1().Services(jaegerNamespace).Get(ctx, jaegerServiceName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		t.Fatal("No load balancer ingress ready for jaeger")
	}

	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:%d", svc.Status.LoadBalancer.Ingress[0].IP, svc.Spec.Ports[0].Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	queryClient := jaegerAPI.NewQueryServiceClient(conn)

	request := &jaegerAPI.FindTracesRequest{
		Query: &jaegerAPI.TraceQueryParameters{
			ServiceName:   tracingServiceName,
			OperationName: "ingress",
			Tags:          tags,
			StartTimeMin:  startTime,
			StartTimeMax:  time.Now(),
			SearchDepth:   100,
		},
	}

	t.Logf("Running query: %+v", request)

	traces, err := queryClient.FindTraces(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	spansNumber := 0
	for {
		_, err := traces.Recv()
		if errors.Is(err, io.EOF) {
			err = traces.CloseSend()
			if err != nil {
				t.Fatal(err)
			}
			return spansNumber
		}
		if err != nil {
			t.Fatal(err)
		}

		spansNumber++
	}
}

// getJaegerEndpoint returns the OTLP endpoint for the given protocol.
func getJaegerEndpoint(protocol string) string {
	switch protocol {
	case metrics.ProtocolGRPC:
		return fmt.Sprintf("http://%s.%s.svc:4317", jaegerServiceName, jaegerNamespace)
	case metrics.ProtocolHTTPProtobuf:
		return fmt.Sprintf("http://%s.%s.svc:4318/v1/traces", jaegerServiceName, jaegerNamespace)
	default:
		return ""
	}
}
