package generator

import (
	"kourier/pkg/envoy"

	"go.uber.org/zap"

	"github.com/golang/protobuf/ptypes/wrappers"

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
	endpoints   []*endpoint.Endpoint
	clusters    *ClustersCache
	routes      []*route.Route
	routeConfig []v2.RouteConfiguration
	listeners   []*v2.Listener
	runtimes    []cache.Resource
	ingresses   map[string]*v1alpha1.Ingress
	logger      *zap.SugaredLogger

	// These mappings are helpful to know the caches affected when there's a
	// change in an ingress.
	localVirtualHostsForIngress    VHostsForIngresses
	externalVirtualHostsForIngress VHostsForIngresses
	statusVirtualHost              *route.VirtualHost
	routesForIngress               map[string][]string
	clustersToIngress              map[string][]string
	sniMatchesForIngress           map[string][]*envoy.SNIMatch
}

func NewCaches(logger *zap.SugaredLogger) *Caches {
	return &Caches{
		clusters:                       newClustersCache(logger.Named("cluster-cache")),
		localVirtualHostsForIngress:    make(VHostsForIngresses),
		externalVirtualHostsForIngress: make(VHostsForIngresses),
		routesForIngress:               make(map[string][]string),
		ingresses:                      make(map[string]*v1alpha1.Ingress),
		clustersToIngress:              make(map[string][]string),
		sniMatchesForIngress:           make(map[string][]*envoy.SNIMatch),
		logger:                         logger,
	}
}

func (caches *Caches) GetIngress(ingressName, ingressNamespace string) *v1alpha1.Ingress {
	caches.logger.Debugf("getting ingress: %s/%s", ingressName, ingressNamespace)
	return caches.ingresses[mapKey(ingressName, ingressNamespace)]
}

func (caches *Caches) AddIngress(ingress *v1alpha1.Ingress) {
	caches.logger.Debugf("adding ingress: %s/%s", ingress.Name, ingress.Namespace)
	caches.ingresses[mapKey(ingress.Name, ingress.Namespace)] = ingress
}

func (caches *Caches) DeleteIngress(ingressName, ingressNamespace string) {
	caches.logger.Debugf("deleting ingress: %s/%s", ingressName, ingressNamespace)
	delete(caches.ingresses, mapKey(ingressName, ingressNamespace))
	delete(caches.clustersToIngress, mapKey(ingressName, ingressNamespace))
}

func (caches *Caches) AddCluster(cluster *v2.Cluster, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding cluster %s for ingress %s/%s", cluster.Name, ingressName, ingressNamespace)
	caches.clusters.set(cluster, ingressName, ingressNamespace)
	caches.addClustersForIngress(cluster, ingressName, ingressNamespace)
}

func (caches *Caches) AddRoute(route *route.Route, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding route %s for ingress %s/%s", route.Name, ingressName, ingressNamespace)
	caches.routes = append(caches.routes, route)

	key := mapKey(ingressName, ingressNamespace)
	caches.routesForIngress[key] = append(caches.routesForIngress[key], route.Name)
}

func (caches *Caches) addClustersForIngress(cluster *v2.Cluster, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding cluster %s for ingress %s/%s", cluster.Name, ingressName, ingressNamespace)
	key := mapKey(ingressName, ingressNamespace)

	caches.clustersToIngress[key] = append(
		caches.clustersToIngress[key],
		cluster.Name,
	)
}

func (caches *Caches) AddExternalVirtualHostForIngress(vHost *route.VirtualHost, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding external virtualhost %s for ingress %s/%s", vHost.Name, ingressName,
		ingressNamespace)
	key := mapKey(ingressName, ingressNamespace)

	caches.externalVirtualHostsForIngress[key] = append(
		caches.externalVirtualHostsForIngress[key],
		vHost,
	)
}

func (caches *Caches) AddInternalVirtualHostForIngress(vHost *route.VirtualHost, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding internal virtualhost %s for ingress %s/%s", vHost.Name, ingressName,
		ingressNamespace)

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

	statusVirtualHost := statusVHost(ingresses)
	caches.statusVirtualHost = &statusVirtualHost
}

func (caches *Caches) AddSNIMatch(sniMatch *envoy.SNIMatch, ingressName string, ingressNamespace string) {
	caches.logger.Debugf("adding SNIMatch for ingress %s/%s", ingressName, ingressNamespace)
	key := mapKey(ingressName, ingressNamespace)
	caches.sniMatchesForIngress[key] = append(caches.sniMatchesForIngress[key], sniMatch)
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

	endpoints := make([]cache.Resource, len(caches.endpoints))
	for i := range caches.endpoints {
		caches.logger.Debugf("Adding Endpoint %#v", caches.endpoints[i])
		endpoints[i] = caches.endpoints[i]
	}

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
		endpoints,
		caches.clusters.list(),
		routes,
		listeners,
		caches.runtimes,
	), nil
}

// Note: changes the snapshot version of the caches object
// Notice that the clusters are not deleted. That's handled with the expiration
// time set in the "ClustersCache" struct.
func (caches *Caches) DeleteIngressInfo(ingressName string, ingressNamespace string,
	kubeclient kubeclient.Interface) error {
	var err error
	caches.deleteRoutesForIngress(ingressName, ingressNamespace)
	caches.deleteMappingsForIngress(ingressName, ingressNamespace)

	// Set to expire all the clusters belonging to that Ingress.
	clusters := caches.clustersToIngress[mapKey(ingressName, ingressNamespace)]
	for _, cluster := range clusters {
		caches.clusters.setExpiration(cluster, ingressName, ingressNamespace)
	}

	caches.DeleteIngress(ingressName, ingressNamespace)

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

func (caches *Caches) deleteRoutesForIngress(ingressName string, ingressNamespace string) {
	var newRoutes []*route.Route

	routesForIngress := caches.routesForIngress[mapKey(ingressName, ingressNamespace)]

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
	delete(caches.sniMatchesForIngress, key)
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

func (caches *Caches) sniMatches() []*envoy.SNIMatch {
	var res []*envoy.SNIMatch

	for _, sniMatches := range caches.sniMatchesForIngress {
		res = append(res, sniMatches...)
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
