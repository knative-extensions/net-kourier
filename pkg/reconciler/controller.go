package reconciler

import (
	"context"
	"flag"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"
	"kourier/pkg/kubernetes"
	"os"
	"path/filepath"

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
	kubernetesClient := kubernetes.NewKubernetesClient(config)
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

	ingressInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	endpointsInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	return impl
}
