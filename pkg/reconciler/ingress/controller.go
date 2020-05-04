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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	kubeclient "knative.dev/pkg/client/injection/kube/client"
	endpointsinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	podinformer "knative.dev/pkg/client/injection/kube/informers/core/v1/pod"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/network"
	knativeReconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"
	"knative.dev/serving/pkg/apis/networking"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
	knativeclient "knative.dev/serving/pkg/client/injection/client"
	ingressinformer "knative.dev/serving/pkg/client/injection/informers/networking/v1alpha1/ingress"
	v1alpha1ingress "knative.dev/serving/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/serving/pkg/network/status"

	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/envoy"
	"knative.dev/net-kourier/pkg/generator"
)

const (
	gatewayLabelKey   = "app"
	gatewayLabelValue = "3scale-kourier-gateway"

	globalResyncPeriod = 30 * time.Second
	controllerName     = "KourierController"
	nodeID             = "3scale-kourier-gateway"
	gatewayPort        = 19001
	managementPort     = 18000
)

func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	kubernetesClient := kubeclient.Get(ctx)
	knativeClient := knativeclient.Get(ctx)
	logger := logging.FromContext(ctx)

	ingressInformer := ingressinformer.Get(ctx)
	endpointsInformer := endpointsinformer.Get(ctx)
	podInformer := podinformer.Get(ctx)

	caches := generator.NewCaches(logger.Named("caches"))
	extAuthZConfig := envoy.GetExternalAuthzConfig()

	r := &Reconciler{
		kubeClient:    kubernetesClient,
		knativeClient: knativeClient,
		caches:        caches,
		extAuthz:      extAuthZConfig.Enabled,
	}

	impl := v1alpha1ingress.NewImpl(ctx, r)

	classFilter := knativeReconciler.AnnotationFilterFunc(
		networking.IngressClassAnnotationKey, config.KourierIngressClassName, false,
	)

	resyncNotReady := func() {
		impl.FilteredGlobalResync(func(obj interface{}) bool {
			return classFilter(obj) && !obj.(*v1alpha1.Ingress).Status.IsReady()
		}, ingressInformer.Informer())
	}
	var callbacks = envoy.Callbacks{
		Logger:  logger,
		OnError: resyncNotReady,
	}

	envoyXdsServer := envoy.NewXdsServer(
		gatewayPort,
		managementPort,
		&callbacks,
	)
	r.xdsServer = envoyXdsServer
	go envoyXdsServer.RunManagementServer()

	statusProber := status.NewProber(
		logger.Named("status-manager"),
		NewProbeTargetLister(logger, endpointsInformer.Lister()),
		func(ing *v1alpha1.Ingress) {
			logger.Debugf("Ready callback triggered for ingress: %s/%s", ing.Namespace, ing.Name)
			impl.EnqueueKey(types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name})
		})
	r.statusManager = statusProber
	statusProber.Start(ctx.Done())

	// This global resync could be removed once we move to envoy >= 1.12
	ticker := time.NewTicker(globalResyncPeriod)
	done := ctx.Done()
	go func() {
		for {
			select {
			case <-done:
				logger.Info("GlobalResync stopped.")
				return
			case <-ticker.C:
				logger.Info("GlobalResync triggered.")
				resyncNotReady()
			}
		}
	}()

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

	endpointsTracker := tracker.New(impl.EnqueueKey, controller.GetTrackerLease(ctx))

	ingressTranslator := generator.NewIngressTranslator(
		r.kubeClient, endpointsInformer.Lister(), network.GetClusterDomainName(), endpointsTracker, logger)
	r.ingressTranslator = &ingressTranslator

	// Make sure we initialize a config. Otherwise, there will be no config
	// until a Knative service is deployed. This is important because the
	// gateway pods will not be marked as healthy until they have been able to
	// fetch a config.
	err := r.caches.InitConfig(kubernetesClient, r.extAuthz)
	if err != nil {
		panic(err)
	}

	snapshot, err := r.caches.ToEnvoySnapshot()
	if err != nil {
		panic(err)
	}

	err = r.xdsServer.SetSnapshot(&snapshot, nodeID)
	if err != nil {
		panic(err)
	}

	// Ingresses need to be filtered by ingress class, so Kourier does not
	// react to nor modify ingresses created by other gateways.
	ingressInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: classFilter,
		Handler:    controller.HandleAll(impl.Enqueue),
	})

	endpointsInformer.Informer().AddEventHandler(controller.HandleAll(
		controller.EnsureTypeMeta(
			endpointsTracker.OnChanged,
			v1.SchemeGroupVersion.WithKind("Endpoints"),
		),
	))

	podInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: knativeReconciler.LabelFilterFunc(gatewayLabelKey, gatewayLabelValue, false),
		Handler: cache.ResourceEventHandlerFuncs{
			// Cancel probing when a Pod is deleted
			DeleteFunc: func(obj interface{}) {
				pod, ok := obj.(*v1.Pod)
				if ok {
					statusProber.CancelPodProbing(pod)
				}
			},
		},
	})

	return impl
}
