package ingress

import (
	"context"
	"kourier/pkg/config"
	"kourier/pkg/envoy"

	"k8s.io/apimachinery/pkg/types"

	"knative.dev/serving/pkg/apis/networking"
	"knative.dev/serving/pkg/reconciler"

	"k8s.io/client-go/tools/cache"

	kubeclient "knative.dev/pkg/client/injection/kube/client"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	knativeclient "knative.dev/serving/pkg/client/injection/client"
	ingressinformer "knative.dev/serving/pkg/client/injection/informers/networking/v1alpha1/ingress"
)

const (
	controllerName = "KourierController"
	nodeID         = "3scale-kourier-gateway"
	gatewayPort    = 19001
	managementPort = 18000
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	kubernetesClient := kubeclient.Get(ctx)
	knativeClient := knativeclient.Get(ctx)

	envoyXdsServer := envoy.NewEnvoyXdsServer(
		gatewayPort,
		managementPort,
		kubernetesClient,
		knativeClient,
	)
	go envoyXdsServer.RunManagementServer()
	go envoyXdsServer.RunGateway()

	logger := logging.FromContext(ctx)

	ingressInformer := ingressinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)

	c := &Reconciler{
		IngressLister:   ingressInformer.Lister(),
		EndpointsLister: endpointsInformer.Lister(),
		EnvoyXDSServer:  envoyXdsServer,
		kubeClient:      kubernetesClient,
	}
	impl := controller.NewImpl(c, logger, controllerName)

	// Ingresses need to be filtered by ingress class, so Kourier does not
	// react to nor modify ingresses created by other gateways.
	ingressInformerHandler := cache.FilteringResourceEventHandler{
		FilterFunc: reconciler.AnnotationFilterFunc(
			networking.IngressClassAnnotationKey, config.KourierIngressClassName, false,
		),
		Handler: controller.HandleAll(impl.Enqueue),
	}
	ingressInformer.Informer().AddEventHandler(ingressInformerHandler)

	// In this first version, we are just refreshing the whole config for any
	// endpoint that we receive. So we should always enqueue the same key.
	enqueueFunc := func(obj interface{}) {
		event := types.NamespacedName{
			Name: EndpointChange,
		}
		impl.EnqueueKey(event)
	}

	// We only want to react to endpoints that belong to a Knative serving and
	// are public.
	endpointsInformerHandler := cache.FilteringResourceEventHandler{
		FilterFunc: reconciler.LabelFilterFunc(
			networking.ServiceTypeKey, string(networking.ServiceTypePublic), false,
		),
		Handler: controller.HandleAll(enqueueFunc),
	}

	endpointsInformer.Informer().AddEventHandler(endpointsInformerHandler)

	// Force a first event to make sure we initialize a config. Otherwise, there
	// will be no config until a Knative service is deployed.
	// This is important because the gateway pods will not be marked as healthy
	// until they have been able to fetch a config.
	event := types.NamespacedName{
		Name: FullResync,
	}
	impl.EnqueueKey(event)

	return impl
}
