package knative

import (
	"fmt"
	"os"
	"time"

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

// IngressAccessors returns the Ingresses that have Kourier ingressClass in the Annotations
// We keep this interface to decouple with the actual Ingress object so it could be changed or replaced
// with other iterations.
func (kNativeClient *KNativeClient) IngressAccessors() ([]networkingv1alpha1.IngressAccessor, error) {
	ingresses, err := kNativeClient.ingresses()
	if err != nil {
		return nil, err
	}

	var ingressList []networkingv1alpha1.IngressAccessor

	for i := range ingresses {
		ingressList = append(ingressList, networkingv1alpha1.IngressAccessor(&ingresses[i]))
	}

	return filterByIngressClass(ingressList), nil
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
		var domain string

		if ingress.GetSpec().Visibility == networkingv1alpha1.IngressVisibilityClusterLocal {
			domain = internalDomain
		} else {
			domain = externalDomain
		}

		status.MarkLoadBalancerReady(
			[]networkingv1alpha1.LoadBalancerIngressStatus{
				{
					DomainInternal: domain,
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
		ingress.SetStatus(*status)

		// Handle both types of ingresses
		switch ingress.(type) {
		case *networkingv1alpha1.Ingress:
			in := ingress.(*networkingv1alpha1.Ingress)
			_, err = kNativeClient.NetworkingClient.Ingresses(ingress.GetNamespace()).UpdateStatus(in)
			return err
		default:
			return fmt.Errorf("can't update object, not Ingress")
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

func (kNativeClient *KNativeClient) ingresses() ([]networkingv1alpha1.Ingress, error) {
	list, err := kNativeClient.NetworkingClient.Ingresses(v1.NamespaceAll).List(v1.ListOptions{})

	if err != nil {
		return nil, err
	}

	return list.Items, err
}

func filterByIngressClass(ingressAccessors []networkingv1alpha1.IngressAccessor) []networkingv1alpha1.IngressAccessor {
	var res []networkingv1alpha1.IngressAccessor

	for _, ingressAccessor := range ingressAccessors {
		ingressClass := ingressClass(ingressAccessor)

		if ingressClass == kourierIngressClassName {
			res = append(res, ingressAccessor)
		}
	}

	return res
}

func ingressClass(ingressAccessor networkingv1alpha1.IngressAccessor) string {
	return ingressAccessor.GetObjectMeta().GetAnnotations()[networking.IngressClassAnnotationKey]
}
