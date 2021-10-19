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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
	pkgTest "knative.dev/pkg/test"
)

const (
	kourierControlNamespace  = "knative-serving"
	kourierControlDeployment = "net-kourier-controller"
)

// TestOneTLScerts verifies that the kourier handles HTTPS request by the cert defined by env value.
func TestOneTLScerts(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	hosts := []string{name + ".example.com"}

	secretName, tlsConfig, _ := ingress.CreateTLSSecret(ctx, t, clients, hosts)

	ing, client, _ := ingress.CreateIngressReadyWithTLS(ctx, t, clients, v1alpha1.IngressSpec{
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
	}, tlsConfig)

	secretNamespace := ing.Namespace
	err := updateDeploymentEnv(ctx, clients, secretNamespace, secretName)
	if err != nil {
		t.Errorf("failed to update deployment: %w", err)
	}
	defer func() {
		err := updateDeploymentEnv(ctx, clients, "", "")
		if err != nil {
			t.Errorf("failed to clean up env values: %w", err)
		}
	}()

	if err = pkgTest.WaitForPodListState(
		ctx,
		clients.KubeClient,
		func(p *corev1.PodList) (bool, error) {
			for i := range p.Items {
				pod := &p.Items[i]
				if strings.Contains(pod.Name, kourierControlDeployment) && podReady(pod) {
					for _, e := range pod.Spec.Containers[0].Env {
						if e.Name == "CERTS_SECRET_NAME" && e.Value == secretName {
							return true, nil
						}
					}
				}
			}
			return false, nil
		},
		"WaitForEnvValuesApplied", kourierControlNamespace); err != nil {
		t.Fatalf("Waiting for Pod.List to have env values on pods of %q: %v", kourierControlDeployment, err)
	}
	if err != nil {
		t.Errorf("failed to get deployment: %w", err)
	}

	// Check without TLS.
	ingress.RuntimeRequest(ctx, t, client, "http://"+name+".example.com")

	// Check with TLS.
	ingress.RuntimeRequest(ctx, t, client, "https://"+name+".example.com")

}

// updateDeploymentEnv updates Deployment's env values.
func updateDeploymentEnv(ctx context.Context, clients *test.Clients, certsNamespace, certsName string) error {
	env := []corev1.EnvVar{}

	deploy, err := clients.KubeClient.AppsV1().Deployments(kourierControlNamespace).Get(ctx, kourierControlDeployment, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "CERTS_SECRET_NAMESPACE" {
			e.Value = certsNamespace
		}
		if e.Name == "CERTS_SECRET_NAME" {
			e.Value = certsName
		}
		env = append(env, e)
	}
	deploy.Spec.Template.Spec.Containers[0].Env = env

	_, err = clients.KubeClient.AppsV1().Deployments(kourierControlNamespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err

}

// podReady checks whether pod's Ready status is True.
func podReady(p *corev1.Pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	// No ready status, probably not even running.
	return false
}
