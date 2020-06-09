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
	"sort"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
	pkgTest "knative.dev/pkg/test"
	"knative.dev/serving/test/conformance/ingress"
	"knative.dev/serving/test/e2e"
)

const (
	kourierGatewayDeployment = "3scale-kourier-gateway"
	kourierGatewayLabel      = "app=3scale-kourier-gateway"
	kourierService           = "kourier"
)

// The Kourier Gateway does not have leader election enabled.
// The test ensures that stopping one of the gateway pods doesn't affect user applications.
func TestKourierGatewayHA(t *testing.T) {
	clients := e2e.Setup(t)

	if err := pkgTest.WaitForDeploymentScale(clients.KubeClient, kourierGatewayDeployment, ingressNamespace, haReplicas); err != nil {
		t.Fatalf("Deployment %s not scaled to %d: %v", kourierGatewayDeployment, haReplicas, err)
	}

	svcName, svcPort, svcCancel := ingress.CreateRuntimeService(t, clients, networking.ServicePortNameHTTP1)
	defer svcCancel()

	// Create an Ingress that we will test while restarting Kourier gateway.
	_, _, ingressCancel := ingress.CreateIngressReady(t, clients, createIngressSpec(svcName, svcPort))
	defer ingressCancel()

	url := apis.HTTP(svcName + domain)

	pods, err := clients.KubeClient.Kube.CoreV1().Pods(ingressNamespace).List(metav1.ListOptions{
		LabelSelector: kourierGatewayLabel,
	})
	if err != nil {
		t.Fatal("Failed to get Gateway pods:", err)
	}
	gatewayPod := pods.Items[0].Name

	origEndpoints, err := pkgTest.GetEndpointAddresses(clients.KubeClient, kourierService, ingressNamespace)
	if err != nil {
		t.Fatalf("Unable to get public endpoints for service %s: %v", kourierService, err)
	}

	if err := clients.KubeClient.Kube.CoreV1().Pods(ingressNamespace).Delete(gatewayPod,
		&metav1.DeleteOptions{
			GracePeriodSeconds: ptr.Int64(0),
		}); err != nil {
		t.Fatalf("Failed to delete pod %s: %v", gatewayPod, err)
	}

	// Wait for the killed gateway to disappear from Kourier endpoints.
	if err := pkgTest.WaitForChangedEndpoints(clients.KubeClient, kourierService, ingressNamespace, origEndpoints); err != nil {
		t.Fatal("Failed to wait for the service to update its endpoints:", err)
	}

	assertIngressEventuallyWorks(t, clients, url.URL())

	// Wait for the deployment to scale up again.
	if err := pkgTest.WaitForDeploymentScale(clients.KubeClient, kourierGatewayDeployment, ingressNamespace, haReplicas); err != nil {
		t.Fatalf("Deployment %s failed to scale up: %v", kourierGatewayDeployment, err)
	}

	if err := pkgTest.WaitForServiceEndpoints(clients.KubeClient, kourierService, ingressNamespace, haReplicas); err != nil {
		t.Fatalf("Failed to wait for %d endpoints for service %s: %v", haReplicas, kourierService, err)
	}

	pods, err = clients.KubeClient.Kube.CoreV1().Pods(ingressNamespace).List(metav1.ListOptions{
		LabelSelector: kourierGatewayLabel,
	})
	if err != nil {
		t.Fatal("Failed to get Gateway pods:", err)
	}

	// Sort the pods according to creation timestamp so that we can kill the oldest one. We want to
	// gradually kill both gateway pods.
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].CreationTimestamp.Before(&pods.Items[j].CreationTimestamp) })

	gatewayPod = pods.Items[0].Name // Stop the oldest gateway pod remaining.

	origEndpoints, err = pkgTest.GetEndpointAddresses(clients.KubeClient, kourierService, ingressNamespace)
	if err != nil {
		t.Fatalf("Unable to get public endpoints for service %s: %v", kourierService, err)
	}

	if err := clients.KubeClient.Kube.CoreV1().Pods(ingressNamespace).Delete(gatewayPod,
		&metav1.DeleteOptions{
			GracePeriodSeconds: ptr.Int64(0),
		}); err != nil {
		t.Fatalf("Failed to delete pod %s: %v", gatewayPod, err)
	}

	// Wait for the killed pod to disappear from Kourier endpoints.
	if err := pkgTest.WaitForChangedEndpoints(clients.KubeClient, kourierService, ingressNamespace, origEndpoints); err != nil {
		t.Fatal("Failed to wait for the service to update its endpoints:", err)
	}

	assertIngressEventuallyWorks(t, clients, url.URL())
}
