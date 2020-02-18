package ingress

import (
	"context"
	"kourier/pkg/config"
	"kourier/pkg/envoy"
	"kourier/pkg/generator"
	"kourier/pkg/knative"

	"knative.dev/serving/pkg/apis/networking/v1alpha1"

	v1 "k8s.io/api/core/v1"

	"knative.dev/serving/pkg/apis/networking"
	"knative.dev/serving/pkg/reconciler"

	"k8s.io/client-go/tools/cache"

	kubeclient "knative.dev/pkg/client/injection/kube/client"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	podinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/tracker"

	knativeclient "knative.dev/serving/pkg/client/injection/client"
	ingressinformer "knative.dev/serving/pkg/client/injection/informers/networking/v1alpha1/ingress"
)

const (
	gatewayLabelKey   = "app"
	gatewayLabelValue = "3scale-kourier-gateway"

	controllerName = "KourierController"
	nodeID         = "3scale-kourier-gateway"
	gatewayPort    = 19001
	managementPort = 18000
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	kubernetesClient := kubeclient.Get(ctx)
	knativeClient := knativeclient.Get(ctx)

	envoyXdsServer := envoy.NewXdsServer(
		gatewayPort,
		managementPort,
	)
	go envoyXdsServer.RunManagementServer()
	go envoyXdsServer.RunGateway()

	logger := logging.FromContext(ctx)

	ingressInformer := ingressinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)
	podInformer := podinformer.Get(ctx)

	caches := generator.NewCaches(logger.Named("caches"))

	readyCallback := func(ingress *v1alpha1.Ingress) {
		logger.Debugf("Ready callback triggered for ingress: %s/%s", ingress.Name, ingress.Namespace)
		err := knative.MarkIngressReady(knativeClient, ingress)
		if err != nil {
			logger.Warnf("Failed to update ingress Ready: %v", err)
		}
	}

	statusProber := NewStatusProber(
		logger.Named("status-manager"),
		podInformer.Lister(),
		readyCallback)

	c := &Reconciler{
		IngressLister:   ingressInformer.Lister(),
		EndpointsLister: endpointsInformer.Lister(),
		EnvoyXDSServer:  envoyXdsServer,
		kubeClient:      kubernetesClient,
		CurrentCaches:   caches,
		statusManager:   statusProber,
		logger:          logger.Named("reconciler"),
	}

	statusProber.Start(ctx.Done())

	impl := controller.NewImpl(c, logger, controllerName)
	c.tracker = tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))

	// Make sure we initialize a config. Otherwise, there will be no config
	// until a Knative service is deployed. This is important because the
	// gateway pods will not be marked as healthy until they have been able to
	// fetch a config.
	c.CurrentCaches.AddStatusVirtualHost()
	err := c.CurrentCaches.SetListeners(kubernetesClient)
	if err != nil {
		panic(err)
	}

	snapshot, err := c.CurrentCaches.ToEnvoySnapshot()
	if err != nil {
		panic(err)
	}

	err = c.EnvoyXDSServer.SetSnapshot(&snapshot, nodeID)
	if err != nil {
		panic(err)
	}

	// Ingresses need to be filtered by ingress class, so Kourier does not
	// react to nor modify ingresses created by other gateways.
	ingressInformerHandler := cache.FilteringResourceEventHandler{
		FilterFunc: reconciler.AnnotationFilterFunc(
			networking.IngressClassAnnotationKey, config.KourierIngressClassName, false,
		),
		Handler: controller.HandleAll(impl.Enqueue),
	}

	ingressInformer.Informer().AddEventHandler(ingressInformerHandler)

	endpointsInformer.Informer().AddEventHandler(controller.HandleAll(
		controller.EnsureTypeMeta(
			c.tracker.OnChanged,
			v1.SchemeGroupVersion.WithKind("Endpoints"),
		),
	))

	podInformerHandler := cache.FilteringResourceEventHandler{
		FilterFunc: reconciler.LabelFilterFunc(gatewayLabelKey, gatewayLabelValue, false),
		Handler: cache.ResourceEventHandlerFuncs{
			// Cancel probing when a Pod is deleted
			DeleteFunc: func(obj interface{}) {
				pod, ok := obj.(*v1.Pod)
				if ok {
					statusProber.CancelPodProbing(pod)
				}
			},
		},
	}

	podInformer.Informer().AddEventHandler(podInformerHandler)

	return impl
}
