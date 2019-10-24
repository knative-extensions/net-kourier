package knative

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"knative.dev/serving/pkg/apis/networking"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
	networkingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/networking/v1alpha1"
	servingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"
	"knative.dev/serving/pkg/reconciler"
	"os"
	"time"
)

const (
	internalServiceName     = "kourier-internal"
	externalServiceName     = "kourier-external"
	namespaceEnv            = "KOURIER_NAMESPACE"
	kourierIngressClassName = "kourier.ingress.networking.knative.dev"
)

type KNativeClient struct {
	ServingClient    *servingClientSet.ServingV1alpha1Client
	NetworkingClient *networkingClientSet.NetworkingV1alpha1Client
	KourierNamespace string
}

func NewKnativeClient(config *rest.Config) KNativeClient {
	kourierNamespace := os.Getenv(namespaceEnv)
	if kourierNamespace == "" {
		log.Info("Env KOURIER_NAMESPACE empty, using default: \"knative-serving\"")
		kourierNamespace = "knative-serving"
	}

	servingClient, err := servingClientSet.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	networkingClient, err := networkingClientSet.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	return KNativeClient{ServingClient: servingClient, NetworkingClient: networkingClient, KourierNamespace: kourierNamespace}
}

func (kNativeClient *KNativeClient) Services(namespace string) (*v1alpha1.ServiceList, error) {
	return kNativeClient.ServingClient.Services(namespace).List(v1.ListOptions{})
}

func (kNativeClient *KNativeClient) ClusterIngresses() ([]networkingv1alpha1.ClusterIngress, error) {
	list, err := kNativeClient.NetworkingClient.ClusterIngresses().List(v1.ListOptions{})

	if err != nil {
		return nil, err
	}

	return list.Items, err
}

func (kNativeClient *KNativeClient) Ingresses() ([]networkingv1alpha1.Ingress, error) {
	list, err := kNativeClient.NetworkingClient.Ingresses("").List(v1.ListOptions{})

	if err != nil {
		return nil, err
	}

	return list.Items, err
}

// Pushes an event to the "events" queue received when theres a change in a ClusterIngress is added/deleted/updated.
func (kNativeClient *KNativeClient) WatchChangesInClusterIngress(namespace string, eventsQueue *workqueue.Type, stopChan <-chan struct{}) {

	restClient := kNativeClient.NetworkingClient.RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "clusteringresses", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&networkingv1alpha1.ClusterIngress{},
		time.Second*30, //TODO: Review resync time and adjust.
		cache.FilteringResourceEventHandler{
			FilterFunc: kourierIngressClassNameFilterFunc(),
			Handler: cache.ResourceEventHandlerFuncs{
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
		},
	)

	controller.Run(stopChan)
}

// Pushes an event to the "events" queue received when theres a change in a Ingress is added/deleted/updated.
func (kNativeClient *KNativeClient) WatchChangesInIngress(namespace string, eventsQueue *workqueue.Type, stopChan <-chan struct{}) {

	restClient := kNativeClient.NetworkingClient.RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "ingresses", namespace,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&networkingv1alpha1.Ingress{},
		time.Second*30, //TODO: Review resync time and adjust.
		cache.FilteringResourceEventHandler{
			FilterFunc: kourierIngressClassNameFilterFunc(),
			Handler: cache.ResourceEventHandlerFuncs{
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
		},
	)

	controller.Run(stopChan)
}

func (kNativeClient *KNativeClient) MarkIngressReady(ingress networkingv1alpha1.IngressAccessor) error {
	// TODO: Improve. Currently once we go trough the generation of the envoy cache, we mark the objects as Ready,
	//  but that is not exactly true, it can take a while until envoy exposes the routes. Is there a way to get a "callback" from envoy?
	var err error
	status := ingress.GetStatus()
	if ingress.GetGeneration() != status.ObservedGeneration || !ingress.GetStatus().IsReady() {

		internalDomain := internalServiceName + "." + kNativeClient.KourierNamespace + ".svc.cluster.local"
		externalDomain := externalServiceName + "." + kNativeClient.KourierNamespace + ".svc.cluster.local"

		status.InitializeConditions()
		status.MarkLoadBalancerReady(
			[]networkingv1alpha1.LoadBalancerIngressStatus{
				{
					DomainInternal: externalDomain,
				},
			},
			[]networkingv1alpha1.LoadBalancerIngressStatus{
				{
					DomainInternal: externalDomain,
				},
			},
			[]networkingv1alpha1.LoadBalancerIngressStatus{
				{
					DomainInternal: internalDomain,
				},
			})
		status.MarkNetworkConfigured()
		status.ObservedGeneration = ingress.GetGeneration()
		status.ObservedGeneration = ingress.GetGeneration()
		ingress.SetStatus(*status)

		// Handle both types of ingresses
		switch ingress.(type) {
		case *networkingv1alpha1.ClusterIngress:
			in := ingress.(*networkingv1alpha1.ClusterIngress)
			_, err = kNativeClient.NetworkingClient.ClusterIngresses().UpdateStatus(in)
			return err
		case *networkingv1alpha1.Ingress:
			in := ingress.(*networkingv1alpha1.Ingress)
			_, err = kNativeClient.NetworkingClient.Ingresses(ingress.GetNamespace()).UpdateStatus(in)
			return err
		default:
			return fmt.Errorf("can't update object, not Ingress or ClusterIngress")
		}
	}
	return nil
}

func kourierIngressClassNameFilterFunc() func(interface{}) bool {
	return reconciler.AnnotationFilterFunc(
		networking.IngressClassAnnotationKey,
		kourierIngressClassName,
		true,
	)
}
