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
	"kourier/pkg/config"
	"kourier/pkg/envoy"
	"kourier/pkg/generator"
	"strings"
	"time"

	"knative.dev/pkg/network"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"

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

	globalResyncPeriod = 30 * time.Second
	controllerName     = "KourierController"
	nodeID             = "3scale-kourier-gateway"
	gatewayPort        = 19001
	managementPort     = 18000
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
	extAuthZConfig := addExtAuthz(caches)

	c := &Reconciler{
		IngressLister:  ingressInformer.Lister(),
		EnvoyXDSServer: envoyXdsServer,
		kubeClient:     kubernetesClient,
		knativeClient:  knativeClient,
		CurrentCaches:  caches,
		logger:         logger.Named("reconciler"),
		ExtAuthz:       extAuthZConfig.Enabled,
	}

	impl := controller.NewImpl(c, logger, controllerName)

	readyCallback := func(ing *v1alpha1.Ingress) {
		logger.Debugf("Ready callback triggered for ingress: %s/%s", ing.Namespace, ing.Name)
		impl.EnqueueKey(types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name})
	}

	statusProber := NewStatusProber(
		logger.Named("status-manager"),
		endpointsInformer.Lister(),
		readyCallback)
	c.statusManager = statusProber
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
				impl.FilteredGlobalResync(ingressNotReady, ingressInformer.Informer())
			}
		}
	}()

	c.CurrentCaches.SetOnEvicted(func(key string, value interface{}) {
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
		c.kubeClient, endpointsInformer.Lister(), network.GetClusterDomainName(), endpointsTracker, logger)
	c.ingressTranslator = &ingressTranslator

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
			endpointsTracker.OnChanged,
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

func addExtAuthz(caches *generator.Caches) envoy.ExternalAuthzConfig {
	extAuthZConfig := envoy.GetExternalAuthzConfig()
	if extAuthZConfig.Enabled {
		cluster := extAuthZConfig.GetExtAuthzCluster()
		// This is a special case, as this cluster is not related to an ingress,
		// The Ingress Name and Ingress Namespace are not really used.
		caches.AddClusterForIngress(cluster, "__extAuthZCluster", "_internal")
	}
	return extAuthZConfig
}

func ingressNotReady(obj interface{}) bool {
	ingress := obj.(*v1alpha1.Ingress)
	return !ingress.Status.IsReady()
}
