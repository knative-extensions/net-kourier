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

package ingress

import (
	"context"
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
)

// TestHTTPOption verifies that the Ingress properly handles HTTPOption field.
func TestHTTPOption(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	tests := []struct {
		httpOption v1alpha1.HTTPOption
		code       int
	}{{
		httpOption: v1alpha1.HTTPOptionEnabled,
		code:       http.StatusOK,
	}, {
		httpOption: v1alpha1.HTTPOptionRedirected,
		code:       http.StatusMovedPermanently,
	}}

	hostCode := make(map[string]int, len(tests))
	var client *http.Client
	// Create multiple ingress with different HTTP option at the same time.
	// This makes sure that each Ingress's HTTP option does not effect on globally.
	for _, test := range tests {
		var host string
		host, client = create(ctx, t, clients, test.httpOption)
		hostCode[host] = test.code
	}

	// Request to each Ingress.
	for host, code := range hostCode {
		checkHTTPOption(ctx, t, client, host, code)
	}
}

func create(ctx context.Context, t *testing.T, clients *test.Clients, httpOption v1alpha1.HTTPOption) (string, *http.Client) {
	name, port, _ := CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	hosts := []string{name + ".example.com"}

	secretName, _ := CreateTLSSecret(ctx, t, clients, hosts)

	_, client, _ := CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
		HTTPOption: httpOption,
		Rules: []v1alpha1.IngressRule{{
			Hosts:      hosts,
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
		TLS: []v1alpha1.IngressTLS{{
			Hosts:           hosts,
			SecretName:      secretName,
			SecretNamespace: test.ServingNamespace,
		}},
	})
	return hosts[0], client
}

func checkHTTPOption(ctx context.Context, t *testing.T, client *http.Client, hostname string, code int) {
	// Check with TLS.
	RuntimeRequest(ctx, t, client, "https://"+hostname)

	// Check without TLS.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// Do not follow redirect.
		return http.ErrUseLastResponse
	}
	resp, err := client.Get("http://" + hostname)
	if err != nil {
		t.Fatal("Error making GET request:", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != code {
		t.Errorf("Unexpected status code: %d, wanted %v", resp.StatusCode, code)
		DumpResponse(ctx, t, resp)
	}
}
