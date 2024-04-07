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
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

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

	// Retrieve the gateway pod name
	gatewayPods, err := clients.KubeClient.CoreV1().Pods(gatewayNs).List(ctx, metav1.ListOptions{
		LabelSelector: kourierGatewayLabel,
	})
	if err != nil {
		t.Fatal("Failed to get Gateway pods:", err)
	}
	if len(gatewayPods.Items) != 1 {
		t.Fatal("This test expects exactly 1 gateway pod, found: ", len(gatewayPods.Items))
	}

	gatewayPodName := gatewayPods.Items[0].Name

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

	// Prepare the request to the ingress: this request will wait for slightly less than the drain time configured
	requestTimeout := drainTime - (3 * time.Second)
	reqURL := fmt.Sprintf("http://%s.example.com?initialTimeout=%d", name, requestTimeout.Milliseconds())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		t.Fatal("Error making GET request:", err)
	}

	errs := make(chan error, 1)
	var statusCode int

	// Do the request asynchronously: goroutine will return only once the service has returned a response (after initialTimeout),
	// or early if there is an error
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			errs <- err
			return
		}

		statusCode = resp.StatusCode
		resp.Body.Close()

		errs <- nil
	}()

	// In parallel, delete the gateway pod:
	// the 1 second sleep before the deletion is here to ensure the request has been sent by the goroutine above
	time.Sleep(1 * time.Second)
	if err := clients.KubeClient.CoreV1().Pods(gatewayNs).Delete(ctx, gatewayPodName, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Failed to delete pod %s: %v", gatewayPodName, err)
	}

	// Wait until we get a response from the asynchronous request
	err = <-errs
	if err != nil {
		t.Fatal(err)
	}

	// The gateway has been gracefully shutdown, so the in-flight request finishes with a OK status code
	assert.Equal(t, statusCode, http.StatusOK)
}
