package generator

import (
	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
)

type VHostsForIngresses map[string][]*route.VirtualHost

type Caches struct {
	endpoints []*endpoint.Endpoint
	clusters  ClustersCache
	routes    []*route.Route
	listeners []*v2.Listener
	runtimes  []cache.Resource
	ingresses map[string]*v1alpha1.Ingress

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
		ingresses:                      make(map[string]*v1alpha1.Ingress),
	}

	return caches
}

func (caches *Caches) GetIngress(ingressName, ingressNamespace string) *v1alpha1.Ingress {
	return caches.ingresses[mapKey(ingressName, ingressNamespace)]
}

func (caches *Caches) AddIngress(ingress *v1alpha1.Ingress) {
	caches.ingresses[mapKey(ingress.Name, ingress.Namespace)] = ingress
}

func (caches *Caches) DeleteIngress(ingressName, ingressNamespace string) {
	delete(caches.ingresses, mapKey(ingressName, ingressNamespace))
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

	var ingresses []*v1alpha1.Ingress
	for _, val := range caches.ingresses {
		ingresses = append(ingresses, val)
	}

	ikrs := internalKourierRoutes(ingresses)
	ikvh := internalKourierVirtualHost(ikrs)
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

	// Generate and append the internal kourier route for keeping track of the snapshot id deployed
	// to each envoy
	snapshotVersion, err := caches.getNewSnapshotVersion()
	if err != nil {
		log.Errorf("Failed generating a new Snapshot version: %s", err)
	}

	return cache.NewSnapshot(
		snapshotVersion,
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
	caches.DeleteIngress(ingressName, ingressNamespace)

	newExternalVirtualHosts := caches.externalVirtualHosts()
	newClusterLocalVirtualHosts := caches.clusterLocalVirtualHosts()

	var ingresses []*v1alpha1.Ingress
	for _, val := range caches.ingresses {
		ingresses = append(ingresses, val)
	}

	ikr := internalKourierRoutes(ingresses)
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

func (caches *Caches) getNewSnapshotVersion() (string, error) {
	snapshotVersion, err := uuid.NewUUID()

	if err != nil {
		return "", err
	}

	return snapshotVersion.String(), nil
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
