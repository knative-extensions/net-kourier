package envoy

import (
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/google/uuid"
	kubeclient "k8s.io/client-go/kubernetes"
)

type VHostsForIngresses map[string][]*route.VirtualHost

type Caches struct {
	endpoints []*endpoint.Endpoint
	clusters  ClustersCache
	routes    []*route.Route
	listeners []*v2.Listener
	runtimes  []cache.Resource

	snapshotVersion string

	// These mappings are helpful to know the caches affected when there's a
	// change in an ingress.
	localVirtualHostsForIngress    VHostsForIngresses
	externalVirtualHostsForIngress VHostsForIngresses
	statusVirtualHost              *route.VirtualHost
	routesForIngress               map[string][]string
}

func NewCaches() Caches {
	caches := Caches{
		clusters:                       newClustersCache(),
		localVirtualHostsForIngress:    make(VHostsForIngresses),
		externalVirtualHostsForIngress: make(VHostsForIngresses),
		routesForIngress:               make(map[string][]string),
	}

	_ = caches.setNewSnapshotVersion()

	return caches
}

func (caches *Caches) AddCluster(cluster *v2.Cluster, serviceName string, serviceNamespace string, path string) {
	caches.clusters.set(serviceName, path, serviceNamespace, cluster)
}

func (caches *Caches) AddRoute(route *route.Route, ingressName string, ingressNamespace string) {
	caches.routes = append(caches.routes, route)

	key := mapKey(ingressName, ingressNamespace)
	caches.routesForIngress[key] = append(caches.routesForIngress[key], route.Name)
}

func (caches *Caches) AddExternalVirtualHostForIngress(vHost *route.VirtualHost, ingressName string, ingressNamespace string) {
	key := mapKey(ingressName, ingressNamespace)

	caches.externalVirtualHostsForIngress[key] = append(
		caches.externalVirtualHostsForIngress[key],
		vHost,
	)
}

func (caches *Caches) AddInternalVirtualHostForIngress(vHost *route.VirtualHost, ingressName string, ingressNamespace string) {
	key := mapKey(ingressName, ingressNamespace)

	caches.localVirtualHostsForIngress[key] = append(
		caches.localVirtualHostsForIngress[key],
		vHost,
	)
}

func (caches *Caches) AddStatusVirtualHost() {
	// Generate and append the internal kourier route for keeping track of the snapshot id deployed
	// to each envoy
	_ = caches.setNewSnapshotVersion()
	ikr := internalKourierRoute(caches.snapshotVersion)
	ikvh := internalKourierVirtualHost(ikr)
	caches.statusVirtualHost = &ikvh
}

func (caches *Caches) SetListeners(kubeclient kubeclient.Interface) {
	localVHosts := append(caches.clusterLocalVirtualHosts(), caches.statusVirtualHost)

	listeners := listenersFromVirtualHosts(
		caches.externalVirtualHosts(),
		localVHosts,
		kubeclient,
	)

	caches.listeners = listeners
}

func (caches *Caches) SnapshotVersion() string {
	return caches.snapshotVersion
}

func (caches *Caches) ToEnvoySnapshot() cache.Snapshot {
	endpoints := make([]cache.Resource, len(caches.endpoints))
	for i := range caches.endpoints {
		endpoints[i] = caches.endpoints[i]
	}

	routes := make([]cache.Resource, len(caches.routes))
	for i := range caches.routes {
		routes[i] = caches.routes[i]
	}

	listeners := make([]cache.Resource, len(caches.listeners))
	for i := range caches.listeners {
		listeners[i] = caches.listeners[i]
	}

	return cache.NewSnapshot(
		caches.snapshotVersion,
		endpoints,
		caches.clusters.list(),
		routes,
		listeners,
		caches.runtimes,
	)
}

// Note: changes the snapshot version of the caches object
// Notice that the clusters are not deleted. That's handled with the expiration
// time set in the "ClustersCache" struct.
func (caches *Caches) DeleteIngressInfo(ingressName string, ingressNamespace string, kubeclient kubeclient.Interface) {
	caches.deleteRoutesForIngress(ingressName, ingressNamespace)

	caches.deleteMappingsForIngress(ingressName, ingressNamespace)

	newExternalVirtualHosts := caches.externalVirtualHosts()
	newClusterLocalVirtualHosts := caches.clusterLocalVirtualHosts()

	// Generate and append the internal kourier route for keeping track of the snapshot id deployed
	// to each envoy
	_ = caches.setNewSnapshotVersion()
	ikr := internalKourierRoute(caches.snapshotVersion)
	ikvh := internalKourierVirtualHost(ikr)
	newClusterLocalVirtualHosts = append(newClusterLocalVirtualHosts, &ikvh)

	caches.listeners = listenersFromVirtualHosts(
		newExternalVirtualHosts,
		newClusterLocalVirtualHosts,
		kubeclient,
	)
}

func (caches *Caches) deleteRoutesForIngress(ingressName string, ingressNamespace string) {
	var newRoutes []*route.Route

	routesForIngress, _ := caches.routesForIngress[mapKey(ingressName, ingressNamespace)]

	for _, cachesRoute := range caches.routes {
		if !contains(routesForIngress, cachesRoute.Name) {
			newRoutes = append(newRoutes, cachesRoute)
		}
	}

	caches.routes = newRoutes
}

func (caches *Caches) deleteMappingsForIngress(ingressName string, ingressNamespace string) {
	key := mapKey(ingressName, ingressNamespace)

	delete(caches.routesForIngress, key)
	delete(caches.externalVirtualHostsForIngress, key)
	delete(caches.localVirtualHostsForIngress, key)
}

func (caches *Caches) setNewSnapshotVersion() error {
	snapshotVersion, err := uuid.NewUUID()

	if err != nil {
		return err
	}

	caches.snapshotVersion = snapshotVersion.String()
	return nil
}

func (caches *Caches) externalVirtualHosts() []*route.VirtualHost {
	var res []*route.VirtualHost

	for _, virtualHosts := range caches.externalVirtualHostsForIngress {
		res = append(res, virtualHosts...)
	}

	return res
}

func (caches *Caches) clusterLocalVirtualHosts() []*route.VirtualHost {
	var res []*route.VirtualHost

	for _, virtualHosts := range caches.localVirtualHostsForIngress {
		res = append(res, virtualHosts...)
	}

	return res
}

func mapKey(ingressName string, ingressNamespace string) string {
	return ingressNamespace + "/" + ingressName
}

func contains(slice []string, s string) bool {
	for _, elem := range slice {
		if elem == s {
			return true
		}
	}
	return false
}
