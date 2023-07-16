//go:build e2e
// +build e2e

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

package tracing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	jaegerAPI "github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/discovery"
	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

const (
	testsNamespace   = "serving-tests"
	tracingNamespace = "tracing"
)

// TestTracing verifies that tracing is configured by kourier on the gateway
func TestTracing(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

	startTime := time.Now()

	name, port, _ := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	ing, client, _ := ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts:      []string{name + ".example.com"},
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

	req, err := http.NewRequest("GET", "http://"+name+".example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	assert.Check(t, err == nil)
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	// Waiting more than 10 seconds (with a margin) allows to have 3 spans:
	// The gateway span (from the http request above) and 2 spans from the probes (knative ingress probe and kube probe).
	time.Sleep(12 * time.Second)

	jaegerQueryService, err := clients.KubeClient.CoreV1().Services(tracingNamespace).
		Get(ctx, "jaeger-query", metav1.GetOptions{})
	assert.NilError(t, err)

	conn, err := grpc.Dial(
		fmt.Sprintf("%s:%d",
			jaegerQueryService.Status.LoadBalancer.Ingress[0].IP,
			jaegerQueryService.Spec.Ports[0].Port,
		),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	queryClient := jaegerAPI.NewQueryServiceClient(conn)

	numberOfTracesFromHTTPCalls := getNumberOfTraces(ctx, t, queryClient,
		"kourier-knative",
		name+".example.com",
		map[string]string{
			"http.status_code": "200",
			"http.url":         "http://" + name + ".example.com/",
			"upstream_cluster": testsNamespace + "/" + name,
			"user_agent":       fmt.Sprintf("knative.dev/%s/%s", t.Name(), ing.Name),
		},
		startTime,
	)
	assert.Check(t, numberOfTracesFromHTTPCalls == 1)

	numberOfTracesFromKnativeIngressProbe := getNumberOfTraces(ctx, t, queryClient,
		"kourier-knative",
		name+".example.com",
		map[string]string{
			"http.status_code": "200",
			"http.url":         "http://" + name + ".example.com/healthz",
			"upstream_cluster": testsNamespace + "/" + name,
			"user_agent":       header.IngressReadinessUserAgent,
		},
		startTime,
	)
	assert.Check(t, numberOfTracesFromKnativeIngressProbe >= 1)

	kubeVersion, err := discovery.NewDiscoveryClient(clients.KubeClient.DiscoveryV1().RESTClient()).ServerVersion()
	if err != nil {
		t.Fatal(err)
	}

	numberOfTracesFromKubeProbe := getNumberOfTraces(ctx, t, queryClient,
		"kourier-knative",
		config.InternalKourierDomain,
		map[string]string{
			"http.status_code": "200",
			"http.url":         "http://" + config.InternalKourierDomain + "/ready",
			"upstream_cluster": generator.ServiceStatsClusterName,
			"user_agent":       fmt.Sprintf("%s%s.%s", header.KubeProbeUAPrefix, kubeVersion.Major, kubeVersion.Minor),
		},
		startTime,
	)
	assert.Check(t, numberOfTracesFromKubeProbe >= 1)
}

func getNumberOfTraces(ctx context.Context, t *testing.T, queryClient jaegerAPI.QueryServiceClient, serviceName, operationName string, tags map[string]string, startTime time.Time) int {
	t.Helper()

	request := &jaegerAPI.FindTracesRequest{
		Query: &jaegerAPI.TraceQueryParameters{
			ServiceName:   serviceName,
			OperationName: operationName,
			Tags:          tags,
			StartTimeMin:  startTime,
		},
	}

	t.Logf("Running query: %+v\n", request)

	traces, err := queryClient.FindTraces(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	spansNumber := 0

	for {
		span, err := traces.Recv()
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

		t.Logf("Spans: %+v\n", span.Spans)
		spansNumber++
	}
}
