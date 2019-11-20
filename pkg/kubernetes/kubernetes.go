package kubernetes

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func Config(kubeConfigPath string) *rest.Config {
	var config *rest.Config
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		config, _ = rest.InClusterConfig()
	}

	return config
}

func ServiceForRevision(kubeclient kubernetes.Interface, namespace string, revisionName string) (*v1.Service, error) {
	return kubeclient.CoreV1().Services(namespace).Get(revisionName, metav1.GetOptions{})
}

func GetSecret(kubeclient kubernetes.Interface, namespace string, secretName string) (*v1.Secret, error) {
	return kubeclient.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
}
