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
	"errors"
	"sync"

	"go.uber.org/zap"
	"knative.dev/net-kourier/pkg/envoy"

	"github.com/golang/protobuf/ptypes/wrappers"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/google/uuid"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

var ErrDomainConflict = errors.New("ingress has a conflicting domain with another ingress")

type Caches struct {
	mu                  sync.Mutex
	ingresses           map[string]*v1alpha1.Ingress
	translatedIngresses map[string]*translatedIngress
	clusters            *ClustersCache
	clustersToIngress   map[string][]string
	routeConfig         []v2.RouteConfiguration
	listeners           []*v2.Listener
	statusVirtualHost   *route.VirtualHost
	logger              *zap.SugaredLogger
	ingressesToSync     map[string]struct{}
}

func NewCaches(logger *zap.SugaredLogger, kubernetesClient kubeclient.Interface, extAuthz bool, ingressesToSync map[string]struct{}) (*Caches, error) {
	c := &Caches{
		ingresses:           make(map[string]*v1alpha1.Ingress),
		translatedIngresses: make(map[string]*translatedIngress),
		clusters:            newClustersCache(logger.Named("cluster-cache")),
		clustersToIngress:   make(map[string][]string),
		logger:              logger,
		ingressesToSync:     ingressesToSync,
	}
	err := c.initConfig(kubernetesClient, extAuthz)
	return c, err
}

func (caches *Caches) HasSynced() bool {
	return len(caches.ingressesToSync) == 0
}

func (caches *Caches) UpdateIngress(ingress *v1alpha1.Ingress, ingressTranslation *translatedIngress, kubeclient kubeclient.Interface) error {
	// we hold a lock for Updating the ingress, to avoid another worker to generate an snapshot just when we have
	// deleted the ingress before adding it.
	caches.mu.Lock()
	defer caches.mu.Unlock()

	caches.deleteTranslatedIngress(ingress.Name, ingress.Namespace)

	if err := caches.addTranslatedIngress(ingress, ingressTranslation); err != nil {
		return err
	}

	return caches.setListeners(kubeclient)
}

func (caches *Caches) initConfig(kubernetesClient kubeclient.Interface, extAuthz bool) error {
	if extAuthz {
		extAuthZConfig := envoy.GetExternalAuthzConfig()
		caches.addClusterForIngress(extAuthZConfig.Cluster, "__extAuthZCluster", "_internal")
	}
	caches.addStatusVirtualHost()
	return caches.setListeners(kubernetesClient)
}

func (caches *Caches) GetIngress(ingressName, ingressNamespace string) *v1alpha1.Ingress {
	caches.mu.Lock()
	defer caches.mu.Unlock()
	caches.logger.Debugf("getting ingress: %s/%s", ingressName, ingressNamespace)
	return caches.ingresses[MapKey(ingressName, ingressNamespace)]
}

func (caches *Caches) validateIngress(translatedIngress *translatedIngress) error {
	// We compare the Translated Ingress to current cached Virtualhosts, and look for any domain
	// clashes. If there's one clashing domain, we reject the ingress.
	localVhosts := caches.clusterLocalVirtualHosts()

	// Return true early.
	if len(localVhosts) == 0 {
		return nil
	}

	for _, vhost := range translatedIngress.internalVirtualHosts {
		for _, domain := range vhost.Domains {
			for _, cacheVhost := range localVhosts {
				for _, cachedDomain := range cacheVhost.Domains {
					if domain == cachedDomain {
						return ErrDomainConflict
					}
				}
			}
		}
	}

	return nil
}

func (caches *Caches) addTranslatedIngress(ingress *v1alpha1.Ingress, translatedIngress *translatedIngress) error {
	caches.logger.Debugf("adding ingress: %s/%s", ingress.Name, ingress.Namespace)

	if err := caches.validateIngress(translatedIngress); err != nil {
		return err
	}

	key := MapKey(ingress.Name, ingress.Namespace)
	caches.ingresses[key] = ingress
	caches.translatedIngresses[key] = translatedIngress

	// Remove the Ingress from the Sync list as it has been warmed.
	delete(caches.ingressesToSync, key)

	for _, cluster := range translatedIngress.clusters {
		caches.addClusterForIngress(cluster, ingress.Name, ingress.Namespace)
	}

	return nil
}

// SetOnEvicted allows to set a function that will be executed when any key on the cache expires.
func (caches *Caches) SetOnEvicted(f func(string, interface{})) {
	caches.clusters.clusters.OnEvicted(f)
}

func (caches *Caches) addStatusVirtualHost() {
	statusVirtualHost := statusVHost()
	caches.statusVirtualHost = &statusVirtualHost
}

func (caches *Caches) setListeners(kubeclient kubeclient.Interface) error {
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
	caches.mu.Lock()
	defer caches.mu.Unlock()

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
		caches.logger.Errorf("Failed generating a new Snapshot version: %s", err)
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
	caches.mu.Lock()
	defer caches.mu.Unlock()

	// Remove the Ingress from the Sync list as there's no point to wait for it to be synced.
	delete(caches.ingressesToSync, MapKey(ingressName, ingressNamespace))

	caches.deleteTranslatedIngress(ingressName, ingressNamespace)
	return caches.setListeners(kubeclient)
}

func (caches *Caches) deleteTranslatedIngress(ingressName, ingressNamespace string) {
	caches.logger.Debugf("deleting ingress: %s/%s", ingressName, ingressNamespace)

	key := MapKey(ingressName, ingressNamespace)

	// Set to expire all the clusters belonging to that Ingress.
	clusters := caches.clustersToIngress[key]
	for _, cluster := range clusters {
		caches.clusters.setExpiration(cluster, ingressName, ingressNamespace)
	}

	delete(caches.ingresses, key)
	delete(caches.translatedIngresses, key)
	delete(caches.clustersToIngress, key)
}

func (caches *Caches) addClusterForIngress(cluster *v2.Cluster, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding cluster %s for ingress %s/%s", cluster.Name, ingressName, ingressNamespace)

	caches.clusters.set(cluster, ingressName, ingressNamespace)

	key := MapKey(ingressName, ingressNamespace)
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
	res := make([]*route.VirtualHost, 0, len(caches.translatedIngresses))
	for _, translatedIngress := range caches.translatedIngresses {
		res = append(res, translatedIngress.externalVirtualHosts...)
	}

	return res
}

func (caches *Caches) clusterLocalVirtualHosts() []*route.VirtualHost {
	res := make([]*route.VirtualHost, 0, len(caches.translatedIngresses))
	for _, translatedIngress := range caches.translatedIngresses {
		res = append(res, translatedIngress.internalVirtualHosts...)
	}

	return res
}

func (caches *Caches) sniMatches() []*envoy.SNIMatch {
	res := make([]*envoy.SNIMatch, 0, len(caches.translatedIngresses))
	for _, translatedIngress := range caches.translatedIngresses {
		res = append(res, translatedIngress.sniMatches...)
	}

	return res
}

func MapKey(ingressName string, ingressNamespace string) string {
	return ingressNamespace + "/" + ingressName
}
