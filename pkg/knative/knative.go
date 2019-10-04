package knative

import (
	log "github.com/sirupsen/logrus"
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
	ServingClient    *servingClientSet.ServingV1alpha1Client
	NetworkingClient *networkingClientSet.NetworkingV1alpha1Client
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
	return KNativeClient{ServingClient: servingClient, NetworkingClient: networkingClient}
}

func (kNativeClient *KNativeClient) Services(namespace string) (*v1alpha1.ServiceList, error) {
	return kNativeClient.ServingClient.Services(namespace).List(v1.ListOptions{})
}

func (kNativeClient *KNativeClient) ClusterIngresses() ([]networkingv1alpha1.ClusterIngress, error) {

	list, err := kNativeClient.NetworkingClient.ClusterIngresses().List(v1.ListOptions{})

	return list.Items, err
}

func (kNativeClient *KNativeClient) Ingresses() ([]networkingv1alpha1.Ingress, error) {

	list, err := kNativeClient.NetworkingClient.Ingresses("").List(v1.ListOptions{})

	return list.Items, err
}

// Pushes an event to the "events" channel received when theres a change in a ClusterIngress is added/deleted/updated.
func (kNativeClient *KNativeClient) WatchChangesInClusterIngress(namespace string, events chan<- struct{}, stopChan <-chan struct{}) {

	restClient := kNativeClient.NetworkingClient.RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "clusteringresses", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&networkingv1alpha1.ClusterIngress{},
		time.Second*30, //TODO: Review resync time and adjust.
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				events <- struct{}{}
			},

			DeleteFunc: func(obj interface{}) {
				events <- struct{}{}
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				if oldObj != newObj {
					events <- struct{}{}
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

// Pushes an event to the "events" channel received when theres a change in a Ingress is added/deleted/updated.
func (kNativeClient *KNativeClient) WatchChangesInIngress(namespace string, events chan<- struct{}, stopChan <-chan struct{}) {

	restClient := kNativeClient.NetworkingClient.RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "ingresses", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&networkingv1alpha1.Ingress{},
		time.Second*30, //TODO: Review resync time and adjust.
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				events <- struct{}{}
			},

			DeleteFunc: func(obj interface{}) {
				events <- struct{}{}
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				if oldObj != newObj {
					events <- struct{}{}
				}
			},
		},
	)

	controller.Run(stopChan)
}
