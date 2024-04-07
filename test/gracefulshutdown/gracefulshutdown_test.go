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
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

const kourierGatewayLabel = "app=3scale-kourier-gateway"

func TestGracefulShutdown(t *testing.T) {
	kourierGatewayNamespace = os.Getenv("GATEWAY_NAMESPACE_OVERRIDE")
	if kourierGatewayNamespace == "" {
		t.Fatal("GATEWAY_NAMESPACE_OVERRIDE env var must be set")
	}

	clients := test.Setup(t)
	ctx := context.Background()

	gatewayPods, err := clients.KubeClient.CoreV1().Pods(kourierGatewayNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: kourierGatewayLabel,
	})
	if err != nil {
		t.Fatal("Failed to get Gateway pods:", err)
	}
	if len(gatewayPods) != 1 {
		t.Fatal("This test expects exactly 1 gateway pod, found: ", len(gatewayPods))
	}

	gatewayPodName := pods.Items[0].Name

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

	delay := 20 * time.Second

	var statusCode int

	go func() {
		reqURL := fmt.Sprintf("http://%s.example.com?timeout=%d", name, delay.Milliseconds())
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			t.Fatal("Error making GET request:", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		statusCode = resp.StatusCode
	}()

	time.Sleep(2 * time.Second)

	if err := clients.KubeClient.CoreV1().Pods(kourierGatewayNamespace).Delete(ctx, gatewayPodName, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Failed to delete pod %s: %v", gatewayPodName, err)
	}

	time.Sleep(delay)

	assert.Equal(t, statusCode, http.StatusOK)
}
