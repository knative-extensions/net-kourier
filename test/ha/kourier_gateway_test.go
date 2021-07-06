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

package ha

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
	pkgTest "knative.dev/pkg/test"
)

const (
	kourierGatewayDeployment = "3scale-kourier-gateway"
	kourierGatewayNamespace  = "kourier-system"
	kourierGatewayLabel      = "app=3scale-kourier-gateway"
	kourierService           = "kourier"
)

// The Kourier Gateway does not have leader election enabled.
// The test ensures that stopping one of the gateway pods doesn't affect user applications.
func TestKourierGatewayHA(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

	if err := pkgTest.WaitForDeploymentScale(ctx, clients.KubeClient, kourierGatewayDeployment, kourierGatewayNamespace, haReplicas); err != nil {
		t.Fatalf("Deployment %s not scaled to %d: %v", kourierGatewayDeployment, haReplicas, err)
	}

	t.Log("Creating a service")
	svcName, svcPort, svcCancel := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
	defer svcCancel()

	// Create an Ingress that we will test while restarting Kourier gateway.
	t.Log("Creating the ingress")
	_, _, ingressCancel := ingress.CreateIngressReady(ctx, t, clients, createIngressSpec(svcName, svcPort))
	defer ingressCancel()

	url := apis.HTTP(svcName + domain)

	pods, err := clients.KubeClient.CoreV1().Pods(kourierGatewayNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: kourierGatewayLabel,
	})
	if err != nil {
		t.Fatal("Failed to get Gateway pods:", err)
	}

	for _, pod := range pods.Items {
		t.Logf("Wait for a gateway deployment to be consistent")
		if err := pkgTest.WaitForDeploymentScale(ctx, clients.KubeClient, kourierGatewayDeployment, kourierGatewayNamespace, haReplicas); err != nil {
			t.Fatalf("Deployment %s not scaled to %d: %v", kourierGatewayDeployment, haReplicas, err)
		}
		if err := pkgTest.WaitForServiceEndpoints(ctx, clients.KubeClient, kourierService, kourierGatewayNamespace, haReplicas); err != nil {
			t.Fatalf("Deployment %s failed to scale up: %v", kourierGatewayDeployment, err)
		}

		t.Logf("Deleting gateway %s", pod.Name)
		if err := clients.KubeClient.CoreV1().Pods(kourierGatewayNamespace).Delete(ctx, pod.Name,
			metav1.DeleteOptions{
				GracePeriodSeconds: ptr.Int64(0),
			}); err != nil {
			t.Fatalf("Failed to delete pod %s: %v", pod.Name, err)
		}

		// Wait for the killed gateway to disappear from the endpoints.
		if err := waitForEndpointsState(clients.KubeClient, kourierService, kourierGatewayNamespace, readyEndpointsDoNotContain(pod.Status.PodIP)); err != nil {
			t.Fatal("Failed to wait for the service to update its endpoints:", err)
		}

		assertIngressEventuallyWorks(ctx, t, clients, url.URL())
	}
}

func waitForEndpointsState(client kubernetes.Interface, svcName, svcNamespace string, inState func(*corev1.Endpoints) (bool, error)) error {
	endpointsService := client.CoreV1().Endpoints(svcNamespace)

	return wait.PollImmediate(test.PollInterval, test.PollTimeout, func() (bool, error) {
		endpoint, err := endpointsService.Get(context.Background(), svcName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return inState(endpoint)
	})
}

func readyEndpointsDoNotContain(ip string) func(*corev1.Endpoints) (bool, error) {
	return func(eps *corev1.Endpoints) (bool, error) {
		for _, subset := range eps.Subsets {
			for _, ready := range subset.Addresses {
				if ready.IP == ip {
					return false, nil
				}
			}
		}
		return true, nil
	}
}
