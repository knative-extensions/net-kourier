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

// This is a cache of Envoy clusters
//
// This cache is needed to avoid downtime during Envoy config updates. When we
// send a new config to Envoy we include listeners, routes and clusters. The
// problem is that routes and clusters are not updated atomically by Envoy, so a
// situation like this is possible:
// 1) Kourier sends a config where route R1 points to cluster C1.
// 2) Kourier send a config where route R2 points to cluster C2. R1 and C1 are
// not included because the ingress they belong to have been deleted.
// 3) Envoy can update the clusters before the routes, so internally, it can see
// route R1 pointing to a cluster that no longer exists. If a request comes, it
// will fail.
// Envoy guarantees eventual consistency, but does not guarantee that all the
// elements in the config are updated atomically. Check this for a more detailed
// explanation:
// https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#eventual-consistency-considerations
// The best solution I have found is to include clusters in new configs even if
// they are no longer referenced by the routes of the new config. This cache
// keeps those old cluster for a small period of time.

package generator

import (
	"strings"
	"time"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	gocache "github.com/patrickmn/go-cache"
)

const (
	defaultExpiration      = 15 * time.Second
	defaultCleanupInterval = 1 * time.Minute
)

type ClustersCache struct {
	clusterExpiration time.Duration
	clusters          *gocache.Cache
}

func newClustersCache() *ClustersCache {
	return newClustersCacheWithExpAndCleanupIntervals(defaultExpiration, defaultCleanupInterval)
}

func newClustersCacheWithExpAndCleanupIntervals(expiration time.Duration, cleanupInterval time.Duration) *ClustersCache {
	goCache := gocache.New(gocache.NoExpiration, cleanupInterval)
	return &ClustersCache{clusters: goCache, clusterExpiration: expiration}
}

func (cc *ClustersCache) set(cluster *v3.Cluster, ingressName string, ingressNamespace string) {
	key := key(cluster.GetName(), ingressName, ingressNamespace)
	cc.clusters.Set(key, cluster, gocache.NoExpiration)
}

func (cc *ClustersCache) setExpiration(clusterName string, ingressName string, ingressNamespace string) {
	key := key(clusterName, ingressName, ingressNamespace)
	if cluster, ok := cc.clusters.Get(key); ok {
		cc.clusters.Set(key, cluster, cc.clusterExpiration)
	}
}

func (cc *ClustersCache) list() []cachetypes.Resource {
	res := make([]cachetypes.Resource, 0, cc.clusters.ItemCount())
	for _, cluster := range cc.clusters.Items() {
		res = append(res, cluster.Object.(cachetypes.Resource))
	}

	return res
}

// Using only the cluster name is not enough to ensure uniqueness, that's why we
// use also the ingress info.
func key(clusterName, ingressName, ingressNamespace string) string {
	return strings.Join([]string{clusterName, ingressName, ingressNamespace}, ":")
}

func explodeKey(key string) (string, string, string) {
	keyParts := strings.Split(key, ":")
	return keyParts[0], keyParts[1], keyParts[2]
}
