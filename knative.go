package main

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
	servingv1alpha1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"
	"time"
)

type KNativeClient struct {
	client *servingv1alpha1.ServingV1alpha1Client
}

func NewKnativeClient(config *rest.Config) KNativeClient {
	servingClient, err := servingv1alpha1.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return KNativeClient{client: servingClient}
}

func (kNativeClient *KNativeClient) Services(namespace string) (*v1alpha1.ServiceList, error) {
	return kNativeClient.client.Services(namespace).List(v1.ListOptions{})
}

// Pushes an event to the "events" channel received when an serving is added/deleted/updated.
func (kNativeClient *KNativeClient) WatchChangesInServices(namespace string, events chan<- string, stopChan <-chan struct{}) {
	restClient := kNativeClient.client.RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "services", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&v1alpha1.Service{},
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
