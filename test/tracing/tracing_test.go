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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/discovery"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/http/header"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

const (
	testsNamespace          = "serving-tests"
	kourierControlNamespace = "knative-serving"
	tracingServerAppLabel   = "tracing-backend-server"
)

var tracingServerLogRegex = regexp.MustCompile(`(?P<datetime>\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) (?P<method>[^ ]+) (?P<endpoint>[^ ]+) - (?P<spans>.*)`)

// we keep only fields necessary for comparison
type TracingSpan struct {
	LocalEndpoint struct {
		ServiceName string `json:"serviceName"`
	} `json:"localEndpoint"`
	Name string `json:"name"`
	Tags struct {
		HTTPStatusCode  string `json:"http.status_code"`
		HTTPURL         string `json:"http.url"`
		UpstreamCluster string `json:"upstream_cluster"`
		UserAgent       string `json:"user_agent"`
	} `json:"tags"`
}

// TestTracing verifies that tracing is configured by kourier on the gateway
func TestTracing(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

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
	req.Header.Add("e2e", "tracing")

	startTime := time.Now()

	resp, err := client.Do(req)
	assert.Check(t, err == nil)
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	// waiting more than 10 seconds (with a margin) allows to have 3 spans:
	// the gateway span (from the http request above) and 2 spans from the probes (knative ingress probe and kube probe)
	time.Sleep(12 * time.Second)

	logs := getLogsFromAllPodsInDeployment(ctx, t, clients.KubeClient, kourierControlNamespace, tracingServerAppLabel, startTime)
	spans := parseTracingSpansFromLogs(t, logs)

	kubeVersion, err := discovery.NewDiscoveryClient(clients.KubeClient.DiscoveryV1().RESTClient()).ServerVersion()
	if err != nil {
		t.Fatal(err)
	}

	spansToFind := []TracingSpan{
		{
			LocalEndpoint: struct {
				ServiceName string `json:"serviceName"`
			}{
				ServiceName: "kourier-knative",
			},
			Name: name + ".example.com",
			Tags: struct {
				HTTPStatusCode  string `json:"http.status_code"`
				HTTPURL         string `json:"http.url"`
				UpstreamCluster string `json:"upstream_cluster"`
				UserAgent       string `json:"user_agent"`
			}{
				HTTPStatusCode:  "200",
				HTTPURL:         "http://" + name + ".example.com/",
				UpstreamCluster: testsNamespace + "/" + name,
				UserAgent:       fmt.Sprintf("knative.dev/%s/%s", t.Name(), ing.Name),
			},
		},
		{
			LocalEndpoint: struct {
				ServiceName string `json:"serviceName"`
			}{
				ServiceName: "kourier-knative",
			},
			Name: config.InternalKourierDomain,
			Tags: struct {
				HTTPStatusCode  string `json:"http.status_code"`
				HTTPURL         string `json:"http.url"`
				UpstreamCluster string `json:"upstream_cluster"`
				UserAgent       string `json:"user_agent"`
			}{
				HTTPStatusCode:  "200",
				HTTPURL:         "http://" + config.InternalKourierDomain + "/ready",
				UpstreamCluster: generator.ServiceStatsClusterName,
				UserAgent:       fmt.Sprintf("%s%s.%s", header.KubeProbeUAPrefix, kubeVersion.Major, kubeVersion.Minor),
			},
		},
		{
			LocalEndpoint: struct {
				ServiceName string `json:"serviceName"`
			}{
				ServiceName: "kourier-knative",
			},
			Name: name + ".example.com",
			Tags: struct {
				HTTPStatusCode  string `json:"http.status_code"`
				HTTPURL         string `json:"http.url"`
				UpstreamCluster string `json:"upstream_cluster"`
				UserAgent       string `json:"user_agent"`
			}{
				HTTPStatusCode:  "200",
				HTTPURL:         "http://" + name + ".example.com/healthz",
				UpstreamCluster: testsNamespace + "/" + name,
				UserAgent:       header.IngressReadinessUserAgent,
			},
		},
	}

	assert.Equal(t, containsSpans(t, spans, spansToFind), true)
}

func getLogsFromAllPodsInDeployment(ctx context.Context, t *testing.T, kubeClient kubeclient.Interface, namespace, appLabelValue string, startTime time.Time) []string {
	t.Helper()

	tracingServerPods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + appLabelValue,
	})
	if err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)

	for _, pod := range tracingServerPods.Items {
		logsReq := kubeClient.CoreV1().Pods(namespace).GetLogs(pod.Name, &v1.PodLogOptions{
			SinceTime: &metav1.Time{Time: startTime},
		})
		podLogsReader, err := logsReq.Stream(ctx)
		if err != nil {
			t.Fatal(err)
		}

		podLogs, err := io.ReadAll(podLogsReader)
		if err != nil {
			t.Fatal(err)
		}

		podLogsReader.Close()

		_, err = buf.Write(podLogs)
		if err != nil {
			t.Fatal(err)
		}
	}

	return strings.Split(buf.String(), "\n")
}

func parseTracingSpansFromLogs(t *testing.T, logs []string) []TracingSpan {
	if os.Getenv("TRACING_COLLECTOR_ENDPOINT") == "" {
		t.Fatal("TRACING_COLLECTOR_ENDPOINT must be set")
	}

	tracingSpans := make([]TracingSpan, 0, len(logs))

	for _, log := range logs {
		match := tracingServerLogRegex.FindStringSubmatch(log)

		if len(match) != tracingServerLogRegex.NumSubexp()+1 {
			continue
		}

		result := make(map[string]string)
		for i, name := range tracingServerLogRegex.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = match[i]
			}
		}

		if result["method"] != "POST" && result["endpoint"] != os.Getenv("TRACING_COLLECTOR_ENDPOINT") {
			continue
		}

		spansString, ok := result["spans"]

		if !ok {
			continue
		}

		var spans []TracingSpan
		err := json.Unmarshal([]byte(spansString), &spans)
		if err != nil {
			continue // this happens if the log is not a span
		}

		tracingSpans = append(tracingSpans, spans...)
	}

	return tracingSpans
}

func Test_parseTracingSpansFromLogs(t *testing.T) {
	logs := []string{
		`2023/07/13 09:46:57 Running the tracing backend server.`,
		`2023/07/13 09:49:50 POST /api/v2/spans - [{"annotations":[{"timestamp":1689241786526291, "value":"ss"}], "traceId":"dfdf78be9f9802f3", "id":"dfdf78be9f9802f3", "kind":"SERVER", "timestamp":1689241786518590, "tags":{"user_agent":"knative.dev/TestTracing/tracing-cdnyafeq", "guid:x-request-id":"4d8666a7-fe4d-99dc-90fe-c10c404392ac", "upstream_cluster":"serving-tests/tracing-bdectujw", "upstream_cluster.name":"serving-tests/tracing-bdectujw", "http.method":"GET", "response_size":"14872", "http.status_code":"200", "http.url":"http://tracing-bdectujw.example.com/", "component":"proxy", "node_id":"3scale-kourier-gateway", "response_flags":"-", "request_size":"0", "downstream_cluster":"-", "http.protocol":"HTTP/1.1", "peer.address":"10.244.1.1"}, "localEndpoint":{"port":0, "serviceName":"kourier-knative", "ipv4":"10.244.1.3"}, "name":"tracing-bdectujw.example.com", "duration":7646}]`,
		`2023/07/13 09:49:50 POST /api/v2/spans - [{"duration":589, "localEndpoint":{"ipv4":"10.244.1.3", "port":0, "serviceName":"kourier-knative"}, "annotations":[{"value":"ss", "timestamp":1689241785729605}], "name":"tracing-bdectujw.example.com", "tags":{"request_size":"0", "http.method":"GET", "guid:x-request-id":"48797e12-baff-951b-bea2-d4893d926776", "downstream_cluster":"-", "response_size":"0", "upstream_cluster":"serving-tests/tracing-bdectujw", "user_agent":"Knative-Ingress-Probe", "http.url":"http://tracing-bdectujw.example.com/healthz", "response_flags":"-", "component":"proxy", "http.status_code":"200", "peer.address":"10.244.1.7", "node_id":"3scale-kourier-gateway", "http.protocol":"HTTP/1.1", "upstream_cluster.name":"serving-tests/tracing-bdectujw"}, "traceId":"a22f524cde88a549", "kind":"SERVER", "timestamp":1689241785728943, "id":"a22f524cde88a549"}]`,
		`2023/07/13 09:49:50 POST /api/v2/spans - [{"id":"edb21d2a980a87db", "duration":394, "name":"internalkourier", "timestamp":1689241787200122, "kind":"SERVER", "localEndpoint":{"ipv4":"10.244.1.3", "port":0, "serviceName":"kourier-knative"}, "tags":{"peer.address":"10.244.1.1", "downstream_cluster":"-", "http.method":"GET", "upstream_cluster.name":"service_stats", "response_flags":"-", "guid:x-request-id":"2f741cae-7a3a-979a-9ab4-a11f6cb36e9e", "request_size":"0", "response_size":"5", "component":"proxy", "http.status_code":"200", "node_id":"3scale-kourier-gateway", "http.url":"http://internalkourier/ready", "http.protocol":"HTTP/1.1", "user_agent":"kube-probe/1.24", "upstream_cluster":"service_stats"}, "traceId":"edb21d2a980a87db", "annotations":[{"value":"ss", "timestamp":1689241787200582}]}]`,
		`2023/07/13 09:49:50 POST / - {"not_a_span": true}`,
	}

	spans := parseTracingSpansFromLogs(t, logs)
	assert.Equal(t, len(spans), 3)

	gatewaySpan := TracingSpan{
		LocalEndpoint: struct {
			ServiceName string `json:"serviceName"`
		}{
			ServiceName: "kourier-knative",
		},
		Name: "tracing-bdectujw.example.com",
		Tags: struct {
			HTTPStatusCode  string `json:"http.status_code"`
			HTTPURL         string `json:"http.url"`
			UpstreamCluster string `json:"upstream_cluster"`
			UserAgent       string `json:"user_agent"`
		}{
			HTTPStatusCode:  "200",
			HTTPURL:         "http://tracing-bdectujw.example.com/",
			UpstreamCluster: "serving-tests/tracing-bdectujw",
			UserAgent:       "knative.dev/TestTracing/tracing-cdnyafeq",
		},
	}
	assert.DeepEqual(t, spans[0], gatewaySpan)

	ingressProbeSpan := TracingSpan{
		LocalEndpoint: struct {
			ServiceName string `json:"serviceName"`
		}{
			ServiceName: "kourier-knative",
		},
		Name: "tracing-bdectujw.example.com",
		Tags: struct {
			HTTPStatusCode  string `json:"http.status_code"`
			HTTPURL         string `json:"http.url"`
			UpstreamCluster string `json:"upstream_cluster"`
			UserAgent       string `json:"user_agent"`
		}{
			HTTPStatusCode:  "200",
			HTTPURL:         "http://tracing-bdectujw.example.com/healthz",
			UpstreamCluster: "serving-tests/tracing-bdectujw",
			UserAgent:       "Knative-Ingress-Probe",
		},
	}
	assert.DeepEqual(t, spans[1], ingressProbeSpan)

	kubeProbeSpan := TracingSpan{
		LocalEndpoint: struct {
			ServiceName string `json:"serviceName"`
		}{
			ServiceName: "kourier-knative",
		},
		Name: "internalkourier",
		Tags: struct {
			HTTPStatusCode  string `json:"http.status_code"`
			HTTPURL         string `json:"http.url"`
			UpstreamCluster string `json:"upstream_cluster"`
			UserAgent       string `json:"user_agent"`
		}{
			HTTPStatusCode:  "200",
			HTTPURL:         "http://internalkourier/ready",
			UpstreamCluster: "service_stats",
			UserAgent:       "kube-probe/1.24",
		},
	}
	assert.DeepEqual(t, spans[2], kubeProbeSpan)
}

func containsSpans(t *testing.T, spans []TracingSpan, spansToFind []TracingSpan) bool {
	spansMap := make(map[TracingSpan]bool)

	for _, span := range spans {
		spansMap[span] = true
	}

	spansFound := 0

	for _, spanToFind := range spansToFind {
		if spansMap[spanToFind] {
			spansFound++
		}
	}

	if spansFound != len(spansToFind) {
		// sort to print a nice diff
		sort.Slice(spans, func(i, j int) bool {
			return spans[i].Name < spans[j].Name && spans[i].Tags.HTTPURL < spans[j].Tags.HTTPURL
		})
		sort.Slice(spansToFind, func(i, j int) bool {
			return spansToFind[i].Name < spansToFind[j].Name && spansToFind[i].Tags.HTTPURL < spansToFind[j].Tags.HTTPURL
		})

		t.Logf("Expected %d, got %d spans. Diff: %v", len(spansToFind), spansFound, cmp.Diff(spans, spansToFind))
	}

	return spansFound == len(spansToFind)
}
