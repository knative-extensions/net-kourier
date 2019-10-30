package main

import (
	"kourier/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
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

func KnativeServingClient(kubeConfigPath string) (*servingClientSet.ServingV1alpha1Client, error) {
	config := kubernetes.Config(kubeConfigPath)
	return servingClientSet.NewForConfig(config)
}
