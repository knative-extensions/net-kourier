package ingress

import (
	"context"
	"flag"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"
	"kourier/pkg/kubernetes"
	"os"
	"path/filepath"

	kubeclient "knative.dev/pkg/client/injection/kube/client"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	ingressinformer "knative.dev/serving/pkg/client/injection/informers/networking/v1alpha1/ingress"
)

const (
	controllerName = "KourierController"
	nodeID         = "3scale-kourier-gateway"
	gatewayPort    = 19001
	managementPort = 18000
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func kubeConfigPath() string {
	kubeconfigInArgs := flag.Lookup("kubeconfig").Value.String()

	if kubeconfigInArgs != "" {
		return kubeconfigInArgs
	}

	if home := homeDir(); home != "" {
		return filepath.Join(home, ".kube", "config")
	}

	return ""
}

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	config := kubernetes.Config(kubeConfigPath())
	kubernetesClient := kubeclient.Get(ctx)

	knativeClient := knative.NewKnativeClient(config)

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
	}
	impl := controller.NewImpl(c, logger, controllerName)

	// In this first version, we are just refreshing the whole config for any
	// event that we receive. So we should always enqueue the same key.
	enqueueFunc := func(obj interface{}) {
		impl.EnqueueKey("")
	}

	ingressInformer.Informer().AddEventHandler(controller.HandleAll(enqueueFunc))
	endpointsInformer.Informer().AddEventHandler(controller.HandleAll(enqueueFunc))

	return impl
}
