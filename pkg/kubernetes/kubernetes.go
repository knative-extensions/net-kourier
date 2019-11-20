package kubernetes

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ServiceForRevision(kubeclient kubernetes.Interface, namespace string, revisionName string) (*v1.Service, error) {
	return kubeclient.CoreV1().Services(namespace).Get(revisionName, metav1.GetOptions{})
}

func GetSecret(kubeclient kubernetes.Interface, namespace string, secretName string) (*v1.Secret, error) {
	return kubeclient.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
}
