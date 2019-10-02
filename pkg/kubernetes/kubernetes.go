package kubernetes

import (
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
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

// Pushes an event to the "events" channel received when an endpoint is added/deleted/updated.
func (kubernetesClient *KubernetesClient) WatchChangesInEndpoints(namespace string, events chan<- string, stopChan <-chan struct{}) {
	restClient := kubernetesClient.Client.CoreV1().RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "endpoints", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&v1.Endpoints{},
		time.Second*30,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				events <- "change"
			},

			DeleteFunc: func(obj interface{}) {
				events <- "change"
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				if oldObj != newObj {
					events <- "change"
				}
			},
		},
	)

	// Wait until caches are sync'd to avoid receiving many events at boot
	sync := cache.WaitForCacheSync(stopChan, controller.HasSynced)
	if !sync {
		log.Error("Error while waiting for caches sync")
	}

	controller.Run(stopChan)
}
