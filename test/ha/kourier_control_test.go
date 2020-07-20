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
	"testing"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
	pkgTest "knative.dev/pkg/test"
	pkgHa "knative.dev/pkg/test/ha"
)

const (
	ingressNamespace         = "kourier-system"
	kourierControlDeployment = "3scale-kourier-control"
)

func TestKourierControlHA(t *testing.T) {
	clients := test.Setup(t)

	if err := pkgTest.WaitForDeploymentScale(clients.KubeClient, kourierControlDeployment, ingressNamespace, haReplicas); err != nil {
		t.Fatalf("Deployment %s not scaled to %d: %v", kourierControlDeployment, haReplicas, err)
	}

	// TODO(mattmoor): Once we switch to the new sharded leader election, we should use more than a single bucket here, but the test is still interesting.
	leaders, err := pkgHa.WaitForNewLeaders(t, clients.KubeClient, kourierControlDeployment, ingressNamespace, sets.NewString(), 1 /* numBuckets */)
	if err != nil {
		t.Fatalf("Failed to get leader: %v", err)
	}
	t.Logf("Got initial leader set: %v", leaders)

	svcName, svcPort, svcCancel := ingress.CreateRuntimeService(t, clients, networking.ServicePortNameHTTP1)
	defer svcCancel()

	_, _, ingressCancel := ingress.CreateIngressReady(t, clients, createIngressSpec(svcName, svcPort))
	defer ingressCancel()

	url := apis.HTTP(svcName + domain)
	prober := test.RunRouteProber(t.Logf, clients, url.URL())
	defer test.AssertProberDefault(t, prober)

	for _, leader := range leaders.List() {
		if err := clients.KubeClient.Kube.CoreV1().Pods(ingressNamespace).Delete(leader, &metav1.DeleteOptions{
			GracePeriodSeconds: ptr.Int64(0),
		}); err != nil && !apierrs.IsNotFound(err) {
			t.Fatalf("Failed to delete pod %s: %v", leader, err)
		}

		if err := pkgTest.WaitForPodDeleted(clients.KubeClient, leader, ingressNamespace); err != nil {
			t.Fatalf("Did not observe %s to actually be deleted: %v", leader, err)
		}
	}

	// Wait for all of the old leaders to go away, and then for the right number to be back.
	if _, err := pkgHa.WaitForNewLeaders(t, clients.KubeClient, kourierControlDeployment, ingressNamespace, leaders, 1 /* numBuckets */); err != nil {
		t.Fatalf("Failed to find new leader: %v", err)
	}

	// Create a new service after electing the new leader to together with a new ingress.
	newSvcName, newSvcPort, newSvcCancel := ingress.CreateRuntimeService(t, clients, networking.ServicePortNameHTTP1)
	defer newSvcCancel()

	_, _, newIngressCancel := ingress.CreateIngressReady(t, clients, createIngressSpec(newSvcName, newSvcPort))
	defer newIngressCancel()

	// Verify the new service is accessible via the ingress.
	assertIngressEventuallyWorks(t, clients, apis.HTTP(newSvcName+domain).URL())
}
