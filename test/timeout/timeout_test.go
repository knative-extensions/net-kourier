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

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

func TestIdleTimeout(t *testing.T) {
	clients := test.Setup(t)
	ctx := context.Background()
	name, port, _ := ingress.CreateTimeoutService(ctx, t, clients)

	// Create a simple Ingress over the Service.
	_, client, _ := ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
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

	const waitDuration = 100 * time.Second

	test := struct {
		name         string
		initialDelay time.Duration
		delay        time.Duration
	}{
		name:         "100s delay before response",
		initialDelay: waitDuration,
	}

	checkTimeout(t, client, name, test.initialDelay, test.delay)

}

func checkTimeout(t *testing.T, client *http.Client, name string, initial time.Duration, timeout time.Duration) {
	reqURL := fmt.Sprintf("http://%s.example.com?initialTimeout=%d&timeout=%d",
		name, initial.Milliseconds(), timeout.Milliseconds())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		t.Fatal("Error making GET request:", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assert.Equal(t, true, resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusGatewayTimeout)
}
