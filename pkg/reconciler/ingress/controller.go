/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ingress

import (
	"context"
	"strings"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/envoy"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	networkingClientSet "knative.dev/networking/pkg/client/clientset/versioned/typed/networking/v1alpha1"
	knativeclient "knative.dev/networking/pkg/client/injection/client"
	ingressinformer "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/ingress"
	v1alpha1ingress "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/networking/pkg/status"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	podinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod"
	serviceinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/service"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	knativeReconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
)

const (
	gatewayLabelKey   = "app"
	gatewayLabelValue = "3scale-kourier-gateway"

	nodeID             = "3scale-kourier-gateway"
	managementPort     = 18000
	cacheWarmUPTimeout = 720 * time.Second
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)

	kubernetesClient := kubeclient.Get(ctx)
	knativeClient := knativeclient.Get(ctx)
	ingressInformer := ingressinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)
	serviceInformer := serviceinformer.Get(ctx)
	podInformer := podinformer.Get(ctx)

	extAuthZConfig := config.GetExternalAuthzConfig()

	// Get the current list of ingresses that are ready, and pass it to the cache so we can
	// know when it has been synced.
	ingressesToSync, err := getReadyIngresses(ctx, knativeClient.NetworkingV1alpha1())
	if err != nil {
		logger.Fatalw("Failed to get initial set of ready ingresses", zap.Error(err))
	}

	// Create a new Cache, with the Readiness endpoint enabled, and the list of current Ingresses.
	caches, err := generator.NewCaches(ctx, logger.Named("caches"), kubernetesClient, extAuthZConfig.Enabled, ingressesToSync)
	if err != nil {
		logger.Fatalw("Failed create new caches", zap.Error(err))
	}

	r := &Reconciler{
		kubeClient: kubernetesClient,
		caches:     caches,
		extAuthz:   extAuthZConfig.Enabled,
	}

	impl := v1alpha1ingress.NewImpl(ctx, r, config.KourierIngressClassName)

	classFilter := knativeReconciler.AnnotationFilterFunc(
		networking.IngressClassAnnotationKey, config.KourierIngressClassName, false,
	)

	envoyXdsServer := envoy.NewXdsServer(managementPort, &envoy.Callbacks{
		OnError: func(req *v2.DiscoveryRequest) {
			logger.Infof("Error pushing snapshot to gateway: code: %v message %s", req.ErrorDetail.Code, req.ErrorDetail.Message)
			impl.FilteredGlobalResync(func(obj interface{}) bool {
				return classFilter(obj) && !obj.(*v1alpha1.Ingress).IsReady()
			}, ingressInformer.Informer())
		},
	})
	r.xdsServer = envoyXdsServer

	statusProber := status.NewProber(
		logger.Named("status-manager"),
		NewProbeTargetLister(logger, endpointsInformer.Lister()),
		func(ing *v1alpha1.Ingress) {
			logger.Debugf("Ready callback triggered for ingress: %s/%s", ing.Namespace, ing.Name)
			impl.EnqueueKey(types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name})
		})
	r.statusManager = statusProber
	statusProber.Start(ctx.Done())

	r.caches.SetOnEvicted(func(key string, value interface{}) {
		// The format of the key received is "clusterName:ingressName:ingressNamespace"
		logger.Debugf("Evicted %s", key)
		keyParts := strings.Split(key, ":")
		// We enqueue the ingress name and namespace as if it was a new event, to force
		// a config refresh.
		impl.EnqueueKey(types.NamespacedName{
			Namespace: keyParts[2],
			Name:      keyParts[1],
		})
	})

	tracker := tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))

	ingressTranslator := generator.NewIngressTranslator(
		r.kubeClient, endpointsInformer.Lister(), serviceInformer.Lister(), tracker)
	r.ingressTranslator = &ingressTranslator

	// Initialize the Envoy snapshot.
	snapshot, err := r.caches.ToEnvoySnapshot()
	if err != nil {
		logger.Fatalw("Failed to create snapshot", zap.Error(err))
	}
	err = r.xdsServer.SetSnapshot(&snapshot, nodeID)
	if err != nil {
		logger.Fatalw("Failed to set snapshot", zap.Error(err))
	}

	// Let's start the management server when our cache is in sync, to avoid sending an incomplete configuration
	// to an already running gateway container. If the cache is not warmed up after "cacheWarmUPTimeout" we just
	// start the server as somehow we couldn't sync.
	go func() {
		waitForCache(logger, caches)
		logger.Info("Starting Management Server on Port ", managementPort)
		if err := envoyXdsServer.RunManagementServer(); err != nil {
			logger.Fatalw("Failed to serve XDS Server", zap.Error(err))
		}
	}()

	// Ingresses need to be filtered by ingress class, so Kourier does not
	// react to nor modify ingresses created by other gateways.
	ingressInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: classFilter,
		Handler:    controller.HandleAll(impl.Enqueue),
	})

	serviceInformer.Informer().AddEventHandler(controller.HandleAll(
		controller.EnsureTypeMeta(
			tracker.OnChanged,
			corev1.SchemeGroupVersion.WithKind("Services"),
		),
	))

	endpointsInformer.Informer().AddEventHandler(controller.HandleAll(
		controller.EnsureTypeMeta(
			tracker.OnChanged,
			corev1.SchemeGroupVersion.WithKind("Endpoints"),
		),
	))

	podInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: knativeReconciler.LabelFilterFunc(gatewayLabelKey, gatewayLabelValue, false),
		Handler: cache.ResourceEventHandlerFuncs{
			// Cancel probing when a Pod is deleted
			DeleteFunc: func(obj interface{}) {
				pod, ok := obj.(*corev1.Pod)
				if ok {
					statusProber.CancelPodProbing(pod)
				}
			},
		},
	})

	return impl
}

func waitForCache(log *zap.SugaredLogger, caches *generator.Caches) {
	timeout := time.After(cacheWarmUPTimeout)
	for {
		select {
		case <-timeout:
			log.Warnf("cache warm up timeout after %s", cacheWarmUPTimeout)
			return
		case <-caches.WaitForSync():
			log.Info("cache is in sync.")
			return
		}
	}
}

func getReadyIngresses(ctx context.Context, knativeClient networkingClientSet.NetworkingV1alpha1Interface) ([]*v1alpha1.Ingress, error) {
	ingresses, err := knativeClient.Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	ingressesToWarm := make([]*v1alpha1.Ingress, 0, len(ingresses.Items))
	for _, ingress := range ingresses.Items {
		if ingress.Annotations[networking.IngressClassAnnotationKey] == config.KourierIngressClassName && ingress.GetStatus().GetCondition(v1alpha1.IngressConditionNetworkConfigured).IsTrue() {
			validIngress := ingress
			ingressesToWarm = append(ingressesToWarm, &validIngress)
		}
	}
	return ingressesToWarm, nil
}
