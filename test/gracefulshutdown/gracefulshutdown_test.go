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

package gracefulshutdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

const (
	kourierGatewayNamespace = "kourier-system"
	kourierGatewayLabel     = "app=3scale-kourier-gateway"
)

func TestGracefulShutdown(t *testing.T) {
	gatewayNs := kourierGatewayNamespace
	if gatewayNsOverride := os.Getenv("GATEWAY_NAMESPACE_OVERRIDE"); gatewayNsOverride != "" {
		gatewayNs = gatewayNsOverride
	}

	// Retrieve drain time from environment
	var drainTime time.Duration
	drainTimeSeconds := os.Getenv("DRAIN_TIME_SECONDS")
	if drainTimeSeconds == "" {
		t.Fatal("DRAIN_TIME_SECONDS environment variable must be set")
	}

	drainTime, err := time.ParseDuration(drainTimeSeconds + "s")
	if err != nil {
		t.Fatal("DRAIN_TIME_SECONDS is an invalid duration:", err)
	}
	if drainTime <= 5*time.Second {
		t.Fatal("DRAIN_TIME_SECONDS must be greater than 5")
	}

	clients := test.Setup(t)
	ctx := context.Background()
	
	// Create a service and an ingress
	name, port, _ := ingress.CreateTimeoutService(ctx, t, clients)
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

	tests := []struct {
		name            string
		requestDuration time.Duration
		wantStatusCode  int
	}{
		{
			name:            fmt.Sprintf("do a request taking slightly less than the drain time: %s", drainTime),
			requestDuration: drainTime - (3 * time.Second),
			wantStatusCode:  http.StatusOK,
		},
		{
			name:            fmt.Sprintf("do a request taking slightly more than the drain time: %s", drainTime),
			requestDuration: drainTime + (3 * time.Second),
			wantStatusCode:  0,
		},
	}

	g := new(errgroup.Group)
	var statusCodes sync.Map

	// Run all requests asynchronously at the same time, and collect the results in statusCodes map
	for i := range tests {
		test := tests[i]

		g.Go(func() error {
			statusCode, err := sendRequest(client, name, test.requestDuration)
			statusCodes.Store(test.name, statusCode)
			return err
		})
	}

	// Ensures the requests sent by the goroutines above are in-flight
	time.Sleep(1 * time.Second)

	// Delete all gateway pods
	gatewayPods, err := clients.KubeClient.CoreV1().Pods(gatewayNs).Delete(ctx, metav1.DeleteOptions{
		LabelSelector: kourierGatewayLabel,
	})
	if err != nil {
		t.Fatal("Failed to get Gateway pods:", err)
	}

	for _, gatewayPod := range gatewayPods.Items {
		if err := clients.KubeClient.CoreV1().Pods(gatewayNs).Delete(ctx, gatewayPod.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Failed to delete pod %s: %v", gatewayPod.Name, err)
		}
	}

	// Wait until we get responses from the asynchronous requests
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		statusCode, _ := statusCodes.Load(test.name)

		assert.Equal(t, statusCode.(int), test.wantStatusCode, fmt.Sprintf("%s has failed: expected %d, got %s",
			test.name, test.wantStatusCode, statusCode))
	}
}

func sendRequest(client *http.Client, name string, requestTimeout time.Duration) (statusCode int, err error) {
	reqURL := fmt.Sprintf("http://%s.example.com?initialTimeout=%d", name, requestTimeout.Milliseconds())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return 0, fmt.Errorf("error making GET request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		var errURL *url.Error
		// When the gateway cuts the connection (after completing the drain process), an EOF error is returned to the client
		if errors.As(err, &errURL) && errors.Is(errURL, io.EOF) {
			return 0, nil
		}

		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}
