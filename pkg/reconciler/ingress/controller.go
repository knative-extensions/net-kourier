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

	v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/server"
	"knative.dev/net-kourier/pkg/generator"
	rconfig "knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	ingressinformer "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/ingress"
	v1alpha1ingress "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	ingresslister "knative.dev/networking/pkg/client/listers/networking/v1alpha1"
	netconfig "knative.dev/networking/pkg/config"
	"knative.dev/networking/pkg/status"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	nsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/namespace"
	podinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod"
	secretfilteredinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/secret/filtered"
	serviceinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/service"
	filteredFactory "knative.dev/pkg/client/injection/kube/informers/factory/filtered"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

const (
	gatewayLabelKey   = "app"
	gatewayLabelValue = "3scale-kourier-gateway"

	nodeID         = "3scale-kourier-gateway"
	managementPort = 18000

	unknownWeightedClusterPrefix = "route: unknown weighted cluster '"
)

var isKourierIngress = reconciler.AnnotationFilterFunc(
	v1alpha1ingress.ClassAnnotationKey, config.KourierIngressClassName, false,
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)

	kubernetesClient := kubeclient.Get(ctx)
	ingressInformer := ingressinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)
	serviceInformer := serviceinformer.Get(ctx)
	podInformer := podinformer.Get(ctx)
	secretInformer := getSecretInformer(ctx)
	namespaceInformer := nsinformer.Get(ctx)

	// Create a new Cache, with the Readiness endpoint enabled, and the list of current Ingresses.
	caches, err := generator.NewCaches(ctx, kubernetesClient, config.ExternalAuthz.Enabled)
	if err != nil {
		logger.Fatalw("Failed create new caches", zap.Error(err))
	}

	r := &Reconciler{
		caches:          caches,
		extAuthz:        config.ExternalAuthz.Enabled,
		namespaceLister: namespaceInformer.Lister(),
	}

	impl := v1alpha1ingress.NewImpl(ctx, r, config.KourierIngressClassName, func(impl *controller.Impl) controller.Options {
		configsToResync := []interface{}{
			&netconfig.Config{},
			&config.Kourier{},
		}
		resync := configmap.TypeFilter(configsToResync...)(func(string, interface{}) {
			impl.FilteredGlobalResync(isKourierIngress, ingressInformer.Informer())
		})
		configStore := rconfig.NewStore(logger.Named("config-store"), resync)
		configStore.WatchConfigs(cmw)
		return controller.Options{
			ConfigStore:       configStore,
			PromoteFilterFunc: isKourierIngress,
		}
	})

	r.resyncConflicts = func() {
		impl.FilteredGlobalResync(func(obj interface{}) bool {
			lbReady := obj.(*v1alpha1.Ingress).Status.GetCondition(v1alpha1.IngressConditionLoadBalancerReady).GetReason()
			// Force reconcile all Kourier ingresses that are either not reconciled yet
			// (and thus might end up in a conflict) or already in conflict.
			return isKourierIngress(obj) && (lbReady == "" || lbReady == conflictReason)
		}, ingressInformer.Informer())
	}

	envoyXdsServer := envoy.NewXdsServer(
		managementPort,
		&xds.CallbackFuncs{
			StreamRequestFunc: func(_ int64, req *v3.DiscoveryRequest) error {
				if req.ErrorDetail == nil {
					return nil
				}
				logger.Warnf("Error pushing snapshot to gateway: code: %v message %s", req.ErrorDetail.Code, req.ErrorDetail.Message)

				// We know we can handle this error without a global resync.
				if strings.HasPrefix(req.ErrorDetail.Message, unknownWeightedClusterPrefix) {
					// The error message contains the service name as referenced by the ingress.
					svc := strings.TrimPrefix(strings.TrimSuffix(req.ErrorDetail.Message, "'"), unknownWeightedClusterPrefix)
					ns, name, err := cache.SplitMetaNamespaceKey(svc)
					if err != nil {
						logger.Errorw("Failed to parse service name from error", zap.Error(err))
						return nil
					}

					logger.Infof("Triggering reconcile for all ingresses referencing %q", svc)
					impl.Tracker.OnChanged(&corev1.Service{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Service",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: ns,
							Name:      name,
						},
					})
					return nil
				}

				// Fallback to a global resync of non-ready ingresses for every other error.
				impl.FilteredGlobalResync(func(obj interface{}) bool {
					return isKourierIngress(obj) && !obj.(*v1alpha1.Ingress).IsReady()
				}, ingressInformer.Informer())

				return nil
			},
		},
	)
	r.xdsServer = envoyXdsServer

	statusProber := status.NewProber(
		logger.Named("status-manager"),
		NewProbeTargetLister(logger, endpointsInformer.Lister(), namespaceInformer.Lister()),
		func(ing *v1alpha1.Ingress) {
			logger.Debugf("Ready callback triggered for ingress: %s/%s", ing.Namespace, ing.Name)
			impl.EnqueueKey(types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name})
		})
	r.statusManager = statusProber
	statusProber.Start(ctx.Done())

	r.caches.SetOnEvicted(func(key types.NamespacedName, value interface{}) {
		logger.Debug("Evicted", key.String())
		// We enqueue the ingress name and namespace as if it was a new event, to force
		// a config refresh.
		impl.EnqueueKey(key)
	})

	ingressTranslator := generator.NewIngressTranslator(
		func(ns, name string) (*corev1.Secret, error) {
			return secretInformer.Lister().Secrets(ns).Get(name)
		},
		func(ns, name string) (*corev1.Endpoints, error) {
			return endpointsInformer.Lister().Endpoints(ns).Get(name)
		},
		func(ns, name string) (*corev1.Service, error) {
			return serviceInformer.Lister().Services(ns).Get(name)
		},
		func(name string) (*corev1.Namespace, error) {
			return namespaceInformer.Lister().Get(name)
		},
		impl.Tracker)
	r.ingressTranslator = &ingressTranslator

	// Initialize the Envoy snapshot.
	snapshot, err := r.caches.ToEnvoySnapshot(ctx)
	if err != nil {
		logger.Fatalw("Failed to create snapshot", zap.Error(err))
	}
	err = r.xdsServer.SetSnapshot(nodeID, snapshot)
	if err != nil {
		logger.Fatalw("Failed to set snapshot", zap.Error(err))
	}

	// Get the current list of ingresses that are ready and seed the Envoy config with them.
	ingressesToSync, err := getReadyIngresses(ingressInformer.Lister())
	if err != nil {
		logger.Fatalw("Failed to fetch ready ingresses", zap.Error(err))
	}
	logger.Infof("Priming the config with %d ingresses", len(ingressesToSync))

	startupTranslator := generator.NewIngressTranslator(
		func(ns, name string) (*corev1.Secret, error) {
			return secretInformer.Lister().Secrets(ns).Get(name)
		},
		func(ns, name string) (*corev1.Endpoints, error) {
			return endpointsInformer.Lister().Endpoints(ns).Get(name)
		},
		func(ns, name string) (*corev1.Service, error) {
			return serviceInformer.Lister().Services(ns).Get(name)
		},
		func(name string) (*corev1.Namespace, error) {
			return namespaceInformer.Lister().Get(name)
		},
		impl.Tracker)

	for _, ingress := range ingressesToSync {
		if err := generator.UpdateInfoForIngress(
			ctx, caches, ingress, &startupTranslator, config.ExternalAuthz.Enabled); err != nil {
			logger.Fatalw("Failed prewarm ingress", zap.Error(err))
		}
	}
	// Update the entire batch of ready ingresses at once.
	if err := r.updateEnvoyConfig(ctx); err != nil {
		logger.Fatalw("Failed to set initial envoy config", zap.Error(err))
	}

	// Let's start the management server **after** the configuration has been seeded.
	go func() {
		logger.Info("Starting Management Server on Port ", managementPort)
		if err := envoyXdsServer.RunManagementServer(); err != nil {
			logger.Fatalw("Failed to serve XDS Server", zap.Error(err))
		}
	}()

	// Ingresses need to be filtered by ingress class, so Kourier does not
	// react to nor modify ingresses created by other gateways.
	ingressInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: isKourierIngress,
		Handler:    controller.HandleAll(impl.Enqueue),
	})

	// Make sure trackers are deleted once the observers are removed.
	// Also reconcile all ingresses in conflict once another ingress is removed to
	// unwedge them.
	ingressInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: isKourierIngress,
		Handler: cache.ResourceEventHandlerFuncs{
			DeleteFunc: impl.Tracker.OnDeletedObserver,
		},
	})

	serviceInformer.Informer().AddEventHandler(controller.HandleAll(
		controller.EnsureTypeMeta(
			impl.Tracker.OnChanged,
			corev1.SchemeGroupVersion.WithKind("Service"),
		),
	))

	viaTracker := controller.EnsureTypeMeta(
		impl.Tracker.OnChanged,
		corev1.SchemeGroupVersion.WithKind("Endpoints"))
	endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    viaTracker,
		DeleteFunc: viaTracker,
		UpdateFunc: func(old interface{}, new interface{}) {
			before := readyAddresses(old.(*corev1.Endpoints))
			after := readyAddresses(new.(*corev1.Endpoints))

			// If the ready addresses have not changed, there is no reason for us to
			// reconcile this endpoint, so why bother?
			if before.Equal(after) {
				return
			}

			viaTracker(new)
		},
	})

	secretInformer.Informer().AddEventHandler(controller.HandleAll(
		controller.EnsureTypeMeta(
			impl.Tracker.OnChanged,
			corev1.SchemeGroupVersion.WithKind("Secret"),
		),
	))

	podInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: reconciler.LabelFilterFunc(gatewayLabelKey, gatewayLabelValue, false),
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

func getReadyIngresses(ingressLister ingresslister.IngressLister) ([]*v1alpha1.Ingress, error) {
	ingresses, err := ingressLister.List(labels.SelectorFromSet(map[string]string{
		v1alpha1ingress.ClassAnnotationKey: config.KourierIngressClassName,
	}))
	if err != nil {
		return nil, err
	}
	ingressesToWarm := make([]*v1alpha1.Ingress, 0, len(ingresses))
	for _, ingress := range ingresses {
		if ingress.GetDeletionTimestamp() == nil && // Ignore ingresses that are already marked for deletion.
			ingress.GetStatus().GetCondition(v1alpha1.IngressConditionNetworkConfigured).IsTrue() {
			ingressesToWarm = append(ingressesToWarm, ingress)
		}
	}
	return ingressesToWarm, nil
}

func readyAddresses(eps *corev1.Endpoints) sets.String {
	var count int
	for _, subset := range eps.Subsets {
		count += len(subset.Addresses)
	}

	if count == 0 {
		return nil
	}

	ready := make(sets.String, count)
	for _, subset := range eps.Subsets {
		for _, address := range subset.Addresses {
			ready.Insert(address.IP)
		}
	}

	return ready
}

func getSecretInformer(ctx context.Context) v1.SecretInformer {
	untyped := ctx.Value(filteredFactory.LabelKey{}) // This should always be not nil and have exactly one selector
	return secretfilteredinformer.Get(ctx, untyped.([]string)[0])
}
