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

package main

import (
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
	networkingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/networking/v1alpha1"
	servingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"
)

// Returns a simple "Hello World" Knative Service. It returns "Hello World!"
// for every request.
func ExampleHelloWorldServing() v1alpha1.Service {
	return v1alpha1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "helloworld-go",
		},
		Spec: v1alpha1.ServiceSpec{
			ConfigurationSpec: v1alpha1.ConfigurationSpec{
				Template: &v1alpha1.RevisionTemplateSpec{
					Spec: v1alpha1.RevisionSpec{
						RevisionSpec: servingv1.RevisionSpec{
							PodSpec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Image: "gcr.io/knative-samples/helloworld-go",
									},
								},
							},
						},
					},
				},
			},
		},
		Status: v1alpha1.ServiceStatus{},
	}
}

func GetExtAuthzDeployment(namespace string) appsv1.Deployment {
	replicas := int32(1)
	return appsv1.Deployment{
		TypeMeta: v1.TypeMeta{
			Kind:       "apps/v1",
			APIVersion: "Deployment",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "externalauthz",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "externalauthz",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "externalauthz",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"app": "externalauthz",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "externalauthz",
							Image:           "test_externalauthz:test",
							Resources:       corev1.ResourceRequirements{},
							ImagePullPolicy: "IfNotPresent",
							Ports:           []corev1.ContainerPort{{ContainerPort: 6000}},
							StartupProbe: &corev1.Probe{
								Handler: corev1.Handler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 6000,
										},
										Host: "localhost",
									},
								},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								PeriodSeconds:       5,
								SuccessThreshold:    2,
								FailureThreshold:    5,
							},
						},
					},
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
		},
		Status: appsv1.DeploymentStatus{},
	}
}

func GetExtAuthzService(namespace string) corev1.Service {
	extAuthzService := corev1.Service{
		TypeMeta: v1.TypeMeta{
			Kind:       "v1",
			APIVersion: "Service",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "externalauthz",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:     "http2",
				Protocol: "TCP",
				Port:     6000,
			}},
			Selector: map[string]string{
				"app": "externalauthz",
			},
			Type: "ClusterIP",
		},
		Status: corev1.ServiceStatus{},
	}
	return extAuthzService
}

func KnativeServingClient(kubeConfigPath string) (*servingClientSet.ServingV1alpha1Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)

	if err != nil {
		return nil, err
	}

	return servingClientSet.NewForConfig(config)
}

func KnativeServingNetworkClient(kubeConfigPath string) (*networkingClientSet.NetworkingV1alpha1Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)

	if err != nil {
		return nil, err
	}

	return networkingClientSet.NewForConfig(config)
}

func watchForIngressReady(networkServingClient *networkingClientSet.NetworkingV1alpha1Client,
	serviceName string,
	namespace string,
	events chan<- struct{},
	stopChan <-chan struct{}) {

	restClient := networkServingClient.RESTClient()

	watchlist := cache.NewListWatchFromClient(
		restClient,
		"ingresses",
		namespace,
		fields.Everything(),
	)

	_, controller := cache.NewInformer(
		watchlist,
		&networkingv1alpha1.Ingress{},
		time.Second*1,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				ingress := obj.(*networkingv1alpha1.Ingress)

				if ingress.Name == serviceName && ingress.Status.IsReady() {
					events <- struct{}{}
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				updatedIngress := newObj.(*networkingv1alpha1.Ingress)

				if updatedIngress.Name == serviceName && updatedIngress.Status.IsReady() {
					events <- struct{}{}
				}
			},
		},
	)

	controller.Run(stopChan)
}

func isDeploymentScaledUp(d *appsv1.Deployment) (bool, error) {
	return d.Status.ReadyReplicas >= 1, nil
}
