// This is a cache of Envoy clusters
//
// To figure out the Envoy clusters that we need to set in the Envoy config, we
// check the Ingress. However, there's a case where there are old clusters being
// used that cannot be extracted from the Ingress.
//
// Imagine a scenario where a revision of a Knative Serving is deployed. Later,
// another revision is deployed. Even if the new revision is configured to get
// 100% of the traffic, Envoy will keep the existing connections to the old
// revision for some time. Envoy marks the listeners attending those connections
// as draining. The routing info associated with the old revision no longer
// appears in the Ingress. However, we need to set the old clusters in order to
// be able to attend those connections in "draining" state. Otherwise, requests
// coming from those connections fail with a 5xx "No route" error.

package generator

import (
	"strings"
	"time"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	gocache "github.com/patrickmn/go-cache"
)

const (
	clusterExpiration      = 5 * time.Minute
	defaultExpiration      = gocache.NoExpiration
	defaultCleanupInterval = 10 * time.Minute
)

type ClustersCache struct {
	clusters *gocache.Cache
}

func newClustersCache() ClustersCache {
	goCache := gocache.New(defaultExpiration, defaultCleanupInterval)
	return ClustersCache{clusters: goCache}
}

func newClustersCacheWithExpAndCleanupIntervals(expiration time.Duration, cleanupInterval time.Duration) ClustersCache {
	goCache := gocache.New(expiration, cleanupInterval)
	return ClustersCache{clusters: goCache}
}

func (cc *ClustersCache) set(serviceWithRevisionName string, path string, namespace string, cluster envoycache.Resource) {
	key := key(serviceWithRevisionName, path, namespace)
	cc.clusters.Set(key, cluster, defaultExpiration)
}

func (cc *ClustersCache) setExpiration(clusterName string) {
	if cluster, ok := cc.clusters.Get(clusterName); ok {
		cc.clusters.Set(clusterName, cluster, clusterExpiration)
	}
}

func (cc *ClustersCache) list() []envoycache.Resource {
	var res []envoycache.Resource

	for _, cluster := range cc.clusters.Items() {
		res = append(res, cluster.Object.(envoycache.Resource))
	}

	return res
}

func key(serviceWithRevisionName string, path string, namespace string) string {
	return strings.Join([]string{namespace, serviceWithRevisionName, path}, ":")
}
