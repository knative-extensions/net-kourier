package knative

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"

	networkingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/networking/v1alpha1"
	servingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"

	"time"
)

type KNativeClient struct {
	servingClient    *servingClientSet.ServingV1alpha1Client
	networkingClient *networkingClientSet.NetworkingV1alpha1Client
}

func NewKnativeClient(config *rest.Config) KNativeClient {
	servingClient, err := servingClientSet.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	networkingClient, err := networkingClientSet.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	return KNativeClient{servingClient: servingClient, networkingClient: networkingClient}
}

func (kNativeClient *KNativeClient) Services(namespace string) (*v1alpha1.ServiceList, error) {
	return kNativeClient.servingClient.Services(namespace).List(v1.ListOptions{})
}

func (kNativeClient *KNativeClient) ClusterIngresses() (*networkingv1alpha1.ClusterIngressList, error) {
	return kNativeClient.networkingClient.ClusterIngresses().List(v1.ListOptions{})
}

// Pushes an event to the "events" channel received when theres a change in a ClusterIngress is added/deleted/updated.
func (kNativeClient *KNativeClient) WatchChangesInClusterIngress(namespace string, events chan<- string, stopChan <-chan struct{}) {

	restClient := kNativeClient.networkingClient.RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "clusteringresses", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&networkingv1alpha1.ClusterIngress{},
		time.Second*30, //TODO: Review resync time and adjust.
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
