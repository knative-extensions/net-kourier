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

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/server"
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
	secretinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/secret"
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

	nodeID         = "3scale-kourier-gateway"
	managementPort = 18000
)

var isKourierIngress = knativeReconciler.AnnotationFilterFunc(
	networking.IngressClassAnnotationKey, config.KourierIngressClassName, false,
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	logger := logging.FromContext(ctx)

	kubernetesClient := kubeclient.Get(ctx)
	knativeClient := knativeclient.Get(ctx)
	ingressInformer := ingressinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)
	serviceInformer := serviceinformer.Get(ctx)
	podInformer := podinformer.Get(ctx)
	secretInformer := secretinformer.Get(ctx)

	// Create a new Cache, with the Readiness endpoint enabled, and the list of current Ingresses.
	caches, err := generator.NewCaches(ctx, kubernetesClient, config.ExternalAuthz.Enabled)
	if err != nil {
		logger.Fatalw("Failed create new caches", zap.Error(err))
	}

	r := &Reconciler{
		caches:   caches,
		extAuthz: config.ExternalAuthz.Enabled,
	}

	impl := v1alpha1ingress.NewImpl(ctx, r, config.KourierIngressClassName)

	envoyXdsServer := envoy.NewXdsServer(
		managementPort,
		&envoy.Callbacks{
			OnError: func(req *v2.DiscoveryRequest) {
				logger.Infof("Error pushing snapshot to gateway: code: %v message %s", req.ErrorDetail.Code, req.ErrorDetail.Message)
				impl.FilteredGlobalResync(func(obj interface{}) bool {
					return isKourierIngress(obj) && !obj.(*v1alpha1.Ingress).IsReady()
				}, ingressInformer.Informer())
			},
		},
	)
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

	r.caches.SetOnEvicted(func(key types.NamespacedName, value interface{}) {
		logger.Debug("Evicted", key.String())
		// We enqueue the ingress name and namespace as if it was a new event, to force
		// a config refresh.
		impl.EnqueueKey(key)
	})

	tracker := tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))

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
		tracker)
	r.ingressTranslator = &ingressTranslator

	// Initialize the Envoy snapshot.
	snapshot, err := r.caches.ToEnvoySnapshot(ctx)
	if err != nil {
		logger.Fatalw("Failed to create snapshot", zap.Error(err))
	}
	err = r.xdsServer.SetSnapshot(&snapshot, nodeID)
	if err != nil {
		logger.Fatalw("Failed to set snapshot", zap.Error(err))
	}

	// Get the current list of ingresses that are ready and seed the Envoy config with them.
	ingressesToSync, err := getReadyIngresses(ctx, knativeClient.NetworkingV1alpha1())
	if err != nil {
		logger.Fatalw("Failed to fetch ready ingresses", zap.Error(err))
	}
	logger.Infof("Priming the config with %d ingresses", len(ingressesToSync))

	// The startup translator uses clients instead of listeners to correctly list all
	// resources at startup.
	startupTranslator := generator.NewIngressTranslator(
		func(ns, name string) (*corev1.Secret, error) {
			return kubernetesClient.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		},
		func(ns, name string) (*corev1.Endpoints, error) {
			return kubernetesClient.CoreV1().Endpoints(ns).Get(ctx, name, metav1.GetOptions{})
		},
		func(ns, name string) (*corev1.Service, error) {
			return kubernetesClient.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
		},
		tracker)

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
	ingressInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: isKourierIngress,
		Handler: cache.ResourceEventHandlerFuncs{
			DeleteFunc: tracker.OnDeletedObserver,
		},
	})

	serviceInformer.Informer().AddEventHandler(controller.HandleAll(
		controller.EnsureTypeMeta(
			tracker.OnChanged,
			corev1.SchemeGroupVersion.WithKind("Services"),
		),
	))

	viaTracker := controller.EnsureTypeMeta(
		tracker.OnChanged,
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
			tracker.OnChanged,
			corev1.SchemeGroupVersion.WithKind("Secrets"),
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

func getReadyIngresses(ctx context.Context, knativeClient networkingClientSet.NetworkingV1alpha1Interface) ([]*v1alpha1.Ingress, error) {
	ingresses, err := knativeClient.Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	ingressesToWarm := make([]*v1alpha1.Ingress, 0, len(ingresses.Items))
	for i := range ingresses.Items {
		ingress := &ingresses.Items[i]
		if isKourierIngress(ingress) &&
			ingress.GetDeletionTimestamp() == nil && // Ignore ingresses that are already marked for deletion.
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
