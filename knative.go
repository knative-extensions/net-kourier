package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
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

func ServiceHTTP2Enabled(service v1alpha1.Service) bool {

	container := service.Spec.Template.Spec.GetContainer()
	if len(container.Ports) > 0 {
		if container.Ports[0].Name == "http2" || container.Ports[0].Name == "h2c" {
			return true
		}
	}
	return false
}

func HTTPPortForEndpointSubset(service v1alpha1.Service, subset corev1.EndpointSubset) (uint32, error) {

	desiredPortName := ""
	// This is here for debug and development purpose, remove once we are sure this kinda works.
	if len(service.Spec.Template.Spec.GetContainer().Ports) > 1 {
		log.Infof("service %s has more than 1 container specified", service.Name)
	}

	if ServiceHTTP2Enabled(service) {
		desiredPortName = "http2"
	}

	switch desiredPortName {
	case "http2":
		for _, port := range subset.Ports {
			if port.Name == "http2" || port.Name == "h2c" {
				return uint32(port.Port), nil
			}
		}
	default:
		for _, port := range subset.Ports {
			if port.Name == "http" || port.Name == "http1" {
				return uint32(port.Port), nil
			}
		}

	}

	return 0, fmt.Errorf("http port not found")
}

func DomainsFromService(service *v1alpha1.Service) ([]string, error) {
	var domains []string

	if service.Status.URL == nil {
		return nil, fmt.Errorf("url is empty")
	}
	domains = append(domains, fmt.Sprintf("%s.%s", service.Name, service.Namespace))
	domains = append(domains, fmt.Sprintf("%s.%s.svc", service.Name, service.Namespace))
	domains = append(domains, service.Status.URL.Host)
	domains = append(domains, service.Status.Address.GetURL().Host)

	return domains, nil
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
