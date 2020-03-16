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

package generator

import (
	"github.com/knative/net-kourier/pkg/envoy"

	"go.uber.org/zap"

	"github.com/golang/protobuf/ptypes/wrappers"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
)

type Caches struct {
	ingresses           map[string]*v1alpha1.Ingress
	translatedIngresses map[string]*translatedIngress
	clusters            *ClustersCache
	clustersToIngress   map[string][]string
	routeConfig         []v2.RouteConfiguration
	listeners           []*v2.Listener
	statusVirtualHost   *route.VirtualHost
	logger              *zap.SugaredLogger
}

func NewCaches(logger *zap.SugaredLogger) *Caches {
	return &Caches{
		ingresses:           make(map[string]*v1alpha1.Ingress),
		translatedIngresses: make(map[string]*translatedIngress),
		clusters:            newClustersCache(logger.Named("cluster-cache")),
		clustersToIngress:   make(map[string][]string),
		logger:              logger,
	}
}

func (caches *Caches) GetIngress(ingressName, ingressNamespace string) *v1alpha1.Ingress {
	caches.logger.Debugf("getting ingress: %s/%s", ingressName, ingressNamespace)
	return caches.ingresses[mapKey(ingressName, ingressNamespace)]
}

func (caches *Caches) AddTranslatedIngress(ingress *v1alpha1.Ingress, translatedIngress *translatedIngress) {
	caches.logger.Debugf("adding ingress: %s/%s", ingress.Name, ingress.Namespace)

	key := mapKey(ingress.Name, ingress.Namespace)
	caches.ingresses[key] = ingress
	caches.translatedIngresses[key] = translatedIngress

	for _, cluster := range translatedIngress.clusters {
		caches.AddClusterForIngress(cluster, ingress.Name, ingress.Namespace)
	}
}

// SetOnEvicted allows to set a function that will be executed when any key on the cache expires.
func (caches *Caches) SetOnEvicted(f func(string, interface{})) {
	caches.clusters.clusters.OnEvicted(f)
}

func (caches *Caches) AddStatusVirtualHost() {
	var ingresses []*v1alpha1.Ingress
	for _, val := range caches.ingresses {
		ingresses = append(ingresses, val)
	}

	statusVirtualHost := statusVHost(ingresses)
	caches.statusVirtualHost = &statusVirtualHost
}

func (caches *Caches) SetListeners(kubeclient kubeclient.Interface) error {
	localVHosts := append(caches.clusterLocalVirtualHosts(), caches.statusVirtualHost)

	listeners, err := listenersFromVirtualHosts(
		caches.externalVirtualHosts(),
		localVHosts,
		caches.sniMatches(),
		kubeclient,
		caches,
	)

	if err != nil {
		return err
	}

	caches.listeners = listeners

	return nil
}

func (caches *Caches) ToEnvoySnapshot() (cache.Snapshot, error) {
	caches.logger.Debugf("Preparing Envoy Snapshot")

	// Instead of sending the Routes, we send the RouteConfigs.
	routes := make([]cache.Resource, len(caches.routeConfig))
	for i := range caches.routeConfig {
		// Without this we can generate routes that point to non-existing clusters
		// That causes some "no_cluster" errors in Envoy and the "TestUpdate"
		// in the Knative serving test suite fails sometimes.
		// Ref: https://github.com/knative/serving/blob/f6da03e5dfed78593c4f239c3c7d67c5d7c55267/test/conformance/ingress/update_test.go#L37
		caches.routeConfig[i].ValidateClusters = &wrappers.BoolValue{Value: true}
		caches.logger.Debugf("Adding Route %#v", &caches.routeConfig[i])
		routes[i] = &caches.routeConfig[i]
	}

	listeners := make([]cache.Resource, len(caches.listeners))
	for i := range caches.listeners {
		caches.logger.Debugf("Adding listener %#v", caches.listeners[i])
		listeners[i] = caches.listeners[i]
	}

	// Generate and append the internal kourier route for keeping track of the snapshot id deployed
	// to each envoy
	snapshotVersion, err := caches.getNewSnapshotVersion()
	if err != nil {
		log.Errorf("Failed generating a new Snapshot version: %s", err)
		return cache.Snapshot{}, err
	}

	return cache.NewSnapshot(
		snapshotVersion,
		make([]cache.Resource, 0),
		caches.clusters.list(),
		routes,
		listeners,
		make([]cache.Resource, 0),
	), nil
}

// Note: changes the snapshot version of the caches object
// Notice that the clusters are not deleted. That's handled with the expiration
// time set in the "ClustersCache" struct.
func (caches *Caches) DeleteIngressInfo(ingressName string, ingressNamespace string,
	kubeclient kubeclient.Interface) error {
	var err error
	caches.deleteTranslatedIngress(ingressName, ingressNamespace)

	newExternalVirtualHosts := caches.externalVirtualHosts()
	newClusterLocalVirtualHosts := caches.clusterLocalVirtualHosts()

	var ingresses []*v1alpha1.Ingress
	for _, val := range caches.ingresses {
		ingresses = append(ingresses, val)
	}

	statusVirtualHost := statusVHost(ingresses)
	newClusterLocalVirtualHosts = append(newClusterLocalVirtualHosts, &statusVirtualHost)

	// We now need the cache in the listenersFromVirtualHosts.
	caches.listeners, err = listenersFromVirtualHosts(
		newExternalVirtualHosts,
		newClusterLocalVirtualHosts,
		caches.sniMatches(),
		kubeclient,
		caches,
	)
	if err != nil {
		return err
	}
	return nil
}

func (caches *Caches) deleteTranslatedIngress(ingressName, ingressNamespace string) {
	caches.logger.Debugf("deleting ingress: %s/%s", ingressName, ingressNamespace)

	key := mapKey(ingressName, ingressNamespace)

	// Set to expire all the clusters belonging to that Ingress.
	clusters := caches.clustersToIngress[key]
	for _, cluster := range clusters {
		caches.clusters.setExpiration(cluster, ingressName, ingressNamespace)
	}

	delete(caches.ingresses, key)
	delete(caches.translatedIngresses, key)
	delete(caches.clustersToIngress, key)
}

func (caches *Caches) AddClusterForIngress(cluster *v2.Cluster, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding cluster %s for ingress %s/%s", cluster.Name, ingressName, ingressNamespace)

	caches.clusters.set(cluster, ingressName, ingressNamespace)

	key := mapKey(ingressName, ingressNamespace)
	caches.clustersToIngress[key] = append(
		caches.clustersToIngress[key],
		cluster.Name,
	)
}

func (caches *Caches) getNewSnapshotVersion() (string, error) {
	snapshotVersion, err := uuid.NewUUID()

	if err != nil {
		return "", err
	}

	return snapshotVersion.String(), nil
}

func (caches *Caches) externalVirtualHosts() []*route.VirtualHost {
	var res []*route.VirtualHost

	for _, translatedIngress := range caches.translatedIngresses {
		res = append(res, translatedIngress.externalVirtualHosts...)
	}

	return res
}

func (caches *Caches) clusterLocalVirtualHosts() []*route.VirtualHost {
	var res []*route.VirtualHost

	for _, translatedIngress := range caches.translatedIngresses {
		res = append(res, translatedIngress.internalVirtualHosts...)
	}

	return res
}

func (caches *Caches) sniMatches() []*envoy.SNIMatch {
	var res []*envoy.SNIMatch

	for _, translatedIngress := range caches.translatedIngresses {
		res = append(res, translatedIngress.sniMatches...)
	}

	return res
}

func mapKey(ingressName string, ingressNamespace string) string {
	return ingressNamespace + "/" + ingressName
}
