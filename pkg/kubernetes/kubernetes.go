package kubernetes

import (
	"flag"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"time"
)

const labelPrefix = "serving.knative.dev/revision="

type KubernetesClient struct {
	client *kubernetes.Clientset
}

func NewKubernetesClient(config *rest.Config) KubernetesClient {
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return KubernetesClient{client: k8sClient}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func Config() *rest.Config {
	// Get the config, $HOME/.kube/config
	// TODO: Read from env var
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String(
			"kubeconfig",
			filepath.Join(home, ".kube", "config"),
			"(optional) absolute path to the kubeconfig file",
		)
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	var config *rest.Config
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		config, _ = rest.InClusterConfig()
	}

	return config
}

func (kubernetesClient *KubernetesClient) EndpointsForRevision(namespace string, revisionName string) (*v1.EndpointsList, error) {
	listOptions := meta_v1.ListOptions{LabelSelector: labelPrefix + revisionName}
	return kubernetesClient.client.CoreV1().Endpoints(namespace).List(listOptions)
}

// Pushes an event to the "events" channel received when an endpoint is added/deleted/updated.
func (kubernetesClient *KubernetesClient) WatchChangesInEndpoints(namespace string, events chan<- string, stopChan <-chan struct{}) {
	restClient := kubernetesClient.client.CoreV1().RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "endpoints", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&v1.Endpoints{},
		time.Second*1,
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

	controller.Run(stopChan)
}
