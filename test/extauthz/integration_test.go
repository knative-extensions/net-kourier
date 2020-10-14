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

package extauthz

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"knative.dev/net-kourier/pkg/config"

	"knative.dev/pkg/test"

	v12 "k8s.io/api/core/v1"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	networkingClientSet "knative.dev/networking/pkg/client/clientset/versioned/typed/networking/v1alpha1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1"
)

const namespace string = "default"
const clusterURL string = "http://localhost:8080"
const domain string = "127.0.0.1.nip.io"
const kourierNamespace string = "knative-serving"

func TestKourierIntegration(t *testing.T) {
	t.Run("ExternalAuthz", ExtAuthzScenario)
}

func ExtAuthzScenario(t *testing.T) {
	kubeconfig := flag.Lookup("kubeconfig").Value.String()
	ctx := context.Background()

	servingClient, err := KnativeServingClient(kubeconfig)
	if err != nil {
		t.Fatal(err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatal(err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	servingNetworkClient, err := KnativeServingNetworkClient(kubeconfig)
	if err != nil {
		t.Fatal(err)
	}

	service, err := setupExtAuthzScenario(ctx, kubeClient, servingClient, servingNetworkClient)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare the request, this one should fail.
	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	req, err := http.NewRequest("GET", clusterURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = fmt.Sprintf("%s.%s.%s", service.Name, namespace, domain)
	// Do the request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, resp.StatusCode, http.StatusForbidden)

	// Prepare the request, this one should succeed
	req, err = http.NewRequest("GET", clusterURL+"/success", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = fmt.Sprintf("%s.%s.%s", service.Name, namespace, domain)

	// Do the request
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// The "hello world" service just returns "Hello World!"
	assert.Equal(t, string(respBody), "Hello World!\n")

	err = cleanExtAuthzScenario(ctx, kubeClient, servingClient, service.Name, "externalauthz", "externalauthz")
	if err != nil {
		t.Fatal(err)
	}

}

func setupExtAuthzScenario(ctx context.Context, k8sClient kubernetes.Interface, servingClient *servingClientSet.ServingV1Client,
	networkServingClient *networkingClientSet.NetworkingV1alpha1Client) (*servingv1.Service, error) {

	kubeClient := test.KubeClient{
		Kube: k8sClient,
	}

	service := ExampleHelloWorldServing()
	createdService, err := servingClient.Services(namespace).Create(ctx, &service, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	err = DeployExtAuthzService(ctx, k8sClient, kourierNamespace)
	if err != nil {
		return nil, err
	}
	// Wait for deployments to be ready
	test.WaitForDeploymentState(ctx, &kubeClient, "externalauthz", isDeploymentScaledUp,
		"DeploymentIsScaledUp", kourierNamespace, 120*time.Second)

	// Patch kourier control to add required ENV vars to enable External Authz.
	kourierControlDeployment, err := k8sClient.AppsV1().Deployments(kourierNamespace).Get(ctx, "3scale-kourier-control",
		metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	ExtAuthzHostEnv := v12.EnvVar{
		Name:      config.ExtAuthzHostEnv,
		Value:     "externalauthz:6000",
		ValueFrom: nil,
	}

	ExtAuthzFailureEnv := v12.EnvVar{
		Name:      config.ExtAuthzFailureModeEnv,
		Value:     "false",
		ValueFrom: nil,
	}

	// Add the env vars to the container env list.
	kourierControlDeployment.Spec.Template.Spec.Containers[0].Env = append(kourierControlDeployment.Spec.Template.
		Spec.Containers[0].Env, ExtAuthzHostEnv)
	kourierControlDeployment.Spec.Template.Spec.Containers[0].Env = append(kourierControlDeployment.Spec.Template.
		Spec.Containers[0].Env, ExtAuthzFailureEnv)

	// Update the object.
	_, err = k8sClient.AppsV1().Deployments(kourierNamespace).Update(ctx, kourierControlDeployment, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	// Wait for deployments to be ready
	test.WaitForDeploymentState(ctx, &kubeClient, kourierControlDeployment.GetName(), isDeploymentScaledUp,
		"DeploymentIsScaledUp", kourierNamespace, 120*time.Second)

	time.Sleep(10 * time.Second)

	eventsIngressReady := make(chan struct{})
	stopChan := make(chan struct{})

	// Wait until the service is ready.
	go watchForIngressReady(networkServingClient, service.Name, service.Namespace, eventsIngressReady, stopChan)

	<-eventsIngressReady

	return createdService, nil
}

func cleanExtAuthzScenario(ctx context.Context, kubeClient kubernetes.Interface, servingClient *servingClientSet.ServingV1Client,
	serviceName string, extAuthzServiceName string, extAuthDeploymentName string) error {

	// Restore env vars
	kourierControlDeployment, err := kubeClient.AppsV1().Deployments(kourierNamespace).Get(ctx, "3scale-kourier-control",
		metav1.GetOptions{})

	if err != nil {
		return err
	}

	var finalEnvs []v12.EnvVar
	for _, env := range kourierControlDeployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name != config.ExtAuthzHostEnv && env.Name != config.ExtAuthzFailureModeEnv {
			finalEnvs = append(finalEnvs, env)
		}
	}

	kourierControlDeployment.Spec.Template.Spec.Containers[0].Env = finalEnvs
	_, err = kubeClient.AppsV1().Deployments(kourierNamespace).Update(ctx, kourierControlDeployment, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	// Delete deployments
	err = kubeClient.CoreV1().Services(kourierNamespace).Delete(ctx, extAuthzServiceName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	err = kubeClient.AppsV1().Deployments(kourierNamespace).Delete(ctx, extAuthDeploymentName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	err = servingClient.Services(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	return nil
}

func DeployExtAuthzService(ctx context.Context, kubeClient kubernetes.Interface, namespace string) error {

	extAuthzService := GetExtAuthzService(namespace)
	extAuthzDeployment := GetExtAuthzDeployment(namespace)

	_, err := kubeClient.AppsV1().Deployments(namespace).Create(ctx, &extAuthzDeployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = kubeClient.CoreV1().Services(namespace).Create(ctx, &extAuthzService, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}
