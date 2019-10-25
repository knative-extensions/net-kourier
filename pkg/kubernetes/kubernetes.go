package kubernetes

import (
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"time"
)

const labelPrefix = "serving.knative.dev/revision="

type KubernetesClient struct {
	Client *kubernetes.Clientset
}

func NewKubernetesClient(config *rest.Config) KubernetesClient {
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return KubernetesClient{Client: k8sClient}
}

func Config(kubeConfigPath string) *rest.Config {
	var config *rest.Config
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		config, _ = rest.InClusterConfig()
	}

	return config
}

func (kubernetesClient *KubernetesClient) EndpointsForRevision(namespace string, revisionName string) (*v1.EndpointsList, error) {
	listOptions := meta_v1.ListOptions{LabelSelector: labelPrefix + revisionName}
	return kubernetesClient.Client.CoreV1().Endpoints(namespace).List(listOptions)
}

func (kubernetesClient *KubernetesClient) ServiceForRevision(namespace string, revisionName string) (*v1.Service, error) {
	return kubernetesClient.Client.CoreV1().Services(namespace).Get(revisionName, meta_v1.GetOptions{})
}

func (kubernetesClient *KubernetesClient) GetSecret(namespace string, secretName string) (*v1.Secret, error) {
	return kubernetesClient.Client.CoreV1().Secrets(namespace).Get(secretName, meta_v1.GetOptions{})
}

// Pushes an event to the "events" queue received when an endpoint is added/deleted/updated.
func (kubernetesClient *KubernetesClient) WatchChangesInEndpoints(namespace string, eventsQueue *workqueue.Type, stopChan <-chan struct{}) {
	restClient := kubernetesClient.Client.CoreV1().RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "endpoints", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&v1.Endpoints{},
		time.Second*30,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				eventsQueue.Add(struct{}{})
			},

			DeleteFunc: func(obj interface{}) {
				eventsQueue.Add(struct{}{})
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				if oldObj != newObj {
					eventsQueue.Add(struct{}{})
				}
			},
		},
	)

	controller.Run(stopChan)

}
