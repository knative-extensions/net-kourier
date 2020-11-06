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
	"context"
	"errors"
	"fmt"
	"sync"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/net-kourier/pkg/config"
)

var ErrDomainConflict = errors.New("ingress has a conflicting domain with another ingress")

type Caches struct {
	mu                  sync.Mutex
	translatedIngresses map[types.NamespacedName]*translatedIngress
	clusters            *ClustersCache
	domainsInUse        sets.String
	routeConfig         []*v2.RouteConfiguration
	listeners           []*v2.Listener
	statusVirtualHost   *route.VirtualHost

	kubeClient kubeclient.Interface
}

func NewCaches(ctx context.Context, kubernetesClient kubeclient.Interface, extAuthz bool) (*Caches, error) {
	c := &Caches{
		translatedIngresses: make(map[types.NamespacedName]*translatedIngress),
		clusters:            newClustersCache(),
		domainsInUse:        sets.NewString(),
		statusVirtualHost:   statusVHost(),
		kubeClient:          kubernetesClient,
	}

	if extAuthz {
		c.clusters.set(config.ExternalAuthz.Cluster, "__extAuthZCluster", "_internal")
	}

	if err := c.setListeners(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (caches *Caches) UpdateIngress(ctx context.Context, ingressTranslation *translatedIngress) error {
	// we hold a lock for Updating the ingress, to avoid another worker to generate an snapshot just when we have
	// deleted the ingress before adding it.
	caches.mu.Lock()
	defer caches.mu.Unlock()

	caches.deleteTranslatedIngress(ingressTranslation.name.Name, ingressTranslation.name.Namespace)

	if err := caches.addTranslatedIngress(ingressTranslation); err != nil {
		return err
	}

	return caches.setListeners(ctx)
}

func (caches *Caches) validateIngress(translatedIngress *translatedIngress) error {
	for _, vhost := range translatedIngress.internalVirtualHosts {
		if caches.domainsInUse.HasAny(vhost.Domains...) {
			return ErrDomainConflict
		}
	}

	return nil
}

func (caches *Caches) addTranslatedIngress(translatedIngress *translatedIngress) error {
	if err := caches.validateIngress(translatedIngress); err != nil {
		return err
	}

	for _, vhost := range translatedIngress.internalVirtualHosts {
		caches.domainsInUse.Insert(vhost.Domains...)
	}

	caches.translatedIngresses[translatedIngress.name] = translatedIngress

	for _, cluster := range translatedIngress.clusters {
		caches.clusters.set(cluster, translatedIngress.name.Name, translatedIngress.name.Namespace)
	}

	return nil
}

// SetOnEvicted allows to set a function that will be executed when any key on the cache expires.
func (caches *Caches) SetOnEvicted(f func(types.NamespacedName, interface{})) {
	caches.clusters.clusters.OnEvicted(func(key string, val interface{}) {
		_, name, namespace := explodeKey(key)
		f(types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, val)
	})
}

func (caches *Caches) setListeners(ctx context.Context) error {
	localVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses)+1)
	externalVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses))
	snis := sniMatches{}

	for _, translatedIngress := range caches.translatedIngresses {
		localVHosts = append(localVHosts, translatedIngress.internalVirtualHosts...)
		externalVHosts = append(externalVHosts, translatedIngress.externalVirtualHosts...)

		for _, match := range translatedIngress.sniMatches {
			snis.consume(match)
		}
	}
	// Append the statusHost too.
	localVHosts = append(localVHosts, caches.statusVirtualHost)

	listeners, err := listenersFromVirtualHosts(
		ctx,
		externalVHosts,
		localVHosts,
		snis.list(),
		caches.kubeClient,
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

	// Instead of sending the Routes, we send the RouteConfigs.
	routes := make([]cache.Resource, len(caches.routeConfig))
	for i := range caches.routeConfig {
		// Without this we can generate routes that point to non-existing clusters
		// That causes some "no_cluster" errors in Envoy and the "TestUpdate"
		// in the Knative serving test suite fails sometimes.
		// Ref: https://github.com/knative/serving/blob/f6da03e5dfed78593c4f239c3c7d67c5d7c55267/test/conformance/ingress/update_test.go#L37
		caches.routeConfig[i].ValidateClusters = &wrappers.BoolValue{Value: true}
		routes[i] = caches.routeConfig[i]
	}

	listeners := make([]cache.Resource, len(caches.listeners))
	for i := range caches.listeners {
		listeners[i] = caches.listeners[i]
	}

	// Generate and append the internal kourier route for keeping track of the snapshot id deployed
	// to each envoy
	snapshotVersion, err := caches.getNewSnapshotVersion()
	if err != nil {
		return cache.Snapshot{}, fmt.Errorf("failed to generate a new snapshot version: %w", err)
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
func (caches *Caches) DeleteIngressInfo(ctx context.Context, ingressName string, ingressNamespace string) error {
	caches.mu.Lock()
	defer caches.mu.Unlock()

	caches.deleteTranslatedIngress(ingressName, ingressNamespace)
	return caches.setListeners(ctx)
}

func (caches *Caches) deleteTranslatedIngress(ingressName, ingressNamespace string) {
	key := types.NamespacedName{
		Namespace: ingressNamespace,
		Name:      ingressName,
	}

	// Set to expire all the clusters belonging to that Ingress.
	if translated := caches.translatedIngresses[key]; translated != nil {
		for _, cluster := range translated.clusters {
			caches.clusters.setExpiration(cluster.Name, ingressName, ingressNamespace)
		}

		for _, vhost := range translated.internalVirtualHosts {
			caches.domainsInUse.Delete(vhost.Domains...)
		}

		delete(caches.translatedIngresses, key)
	}
}

func (caches *Caches) getNewSnapshotVersion() (string, error) {
	snapshotVersion, err := uuid.NewUUID()

	if err != nil {
		return "", err
	}

	return snapshotVersion.String(), nil
}
