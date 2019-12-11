package generator

import (
	"kourier/pkg/config"
	"sort"
	"testing"

	"github.com/golang/protobuf/ptypes"

	"gotest.tools/assert"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	httpconnmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeleteIngressInfo(t *testing.T) {
	caches := NewCaches()
	kubeClient := fake.Clientset{}

	// Add info for an ingress
	firstIngressName := "ingress_1"
	firstIngressNamespace := "ingress_1_namespace"
	createTestDataForIngress(
		&caches,
		firstIngressName,
		firstIngressNamespace,
		"cluster_for_ingress_1",
		"route_for_ingress_1",
		"internal_host_for_ingress_1",
		"external_host_for_ingress_1",
		&kubeClient,
	)

	// Add info for a different ingress
	secondIngressName := "ingress_2"
	secondIngressNamespace := "ingress_2_namespace"
	createTestDataForIngress(
		&caches,
		secondIngressName,
		secondIngressNamespace,
		"cluster_for_ingress_2",
		"route_for_ingress_2",
		"internal_host_for_ingress_2",
		"external_host_for_ingress_2",
		&kubeClient,
	)

	// Delete the first ingress
	caches.DeleteIngressInfo(firstIngressName, firstIngressNamespace, &kubeClient)

	// Check that there's only the route of the second ingress
	assert.Equal(t, 1, len(caches.routes))
	assert.Equal(t, "route_for_ingress_2", caches.routes[0].Name)

	// Check that the listeners only have the virtual hosts of the second
	// ingress.
	// Note: Apart from the vHosts that were added explicitly, there's also
	// the one used to verify the snapshot version.
	vHostsNames, err := getVHostsNames(caches.listeners)

	if err != nil {
		t.Fail()
	}

	sort.Strings(vHostsNames)

	expectedNames := []string{
		"internal_host_for_ingress_2",
		"external_host_for_ingress_2",
		config.InternalKourierDomain,
	}
	sort.Strings(expectedNames)

	assert.DeepEqual(t, expectedNames, vHostsNames)
}

func TestDeleteIngressInfoWhenDoesNotExist(t *testing.T) {
	// If the ingress does not exist, nothing should be deleted from the caches
	// instance.

	caches := NewCaches()
	kubeClient := fake.Clientset{}

	// Add info for an ingress
	firstIngressName := "ingress_1"
	firstIngressNamespace := "ingress_1_namespace"
	createTestDataForIngress(
		&caches,
		firstIngressName,
		firstIngressNamespace,
		"cluster_for_ingress_1",
		"route_for_ingress_1",
		"internal_host_for_ingress_1",
		"external_host_for_ingress_1",
		&kubeClient,
	)

	snapshotBeforeDelete, err := caches.ToEnvoySnapshot()
	if err != nil {
		t.FailNow()
	}

	clustersBeforeDelete := snapshotBeforeDelete.Clusters.Items
	routesBeforeDelete := snapshotBeforeDelete.Routes.Items
	listenersBeforeDelete := snapshotBeforeDelete.Listeners.Items

	err = caches.DeleteIngressInfo("non_existing_name", "non_existing_namespace", &kubeClient)
	if err != nil {
		t.FailNow()
	}

	snapshotAfterDelete, err := caches.ToEnvoySnapshot()
	if err != nil {
		t.FailNow()
	}

	clustersAfterDelete := snapshotAfterDelete.Clusters.Items
	routesAfterDelete := snapshotAfterDelete.Routes.Items
	listenersAfterDelete := snapshotAfterDelete.Listeners.Items

	assert.DeepEqual(t, clustersBeforeDelete, clustersAfterDelete)
	assert.DeepEqual(t, routesBeforeDelete, routesAfterDelete)
	assert.DeepEqual(t, listenersBeforeDelete, listenersAfterDelete)
}

// Creates a cluster, a route, and listeners from the given names and
// associates them with the ingress name/namespace received
func createTestDataForIngress(caches *Caches,
	ingressName string,
	ingressNamespace string,
	clusterName string,
	routeName string,
	internalVHostName string,
	externalVHostName string,
	kubeClient kubeclient.Interface) {

	cluster := v2.Cluster{Name: clusterName}
	caches.AddCluster(&cluster, ingressName, ingressNamespace)

	r := route.Route{Name: routeName}
	caches.AddRoute(&r, ingressName, ingressNamespace)

	externalvHost := route.VirtualHost{Name: externalVHostName}
	internalvHost := route.VirtualHost{Name: internalVHostName}

	caches.AddExternalVirtualHostForIngress(&externalvHost, ingressName, ingressNamespace)
	caches.AddInternalVirtualHostForIngress(&internalvHost, ingressName, ingressNamespace)
	caches.AddStatusVirtualHost()
	caches.SetListeners(kubeClient)
}

func getVHostsNames(listeners []*v2.Listener) ([]string, error) {
	var res []string

	for _, listener := range listeners {
		filterConfig := listener.GetFilterChains()[0].Filters[0].GetTypedConfig()
		httpConnManager := httpconnmanagerv2.HttpConnectionManager{}
		err := ptypes.UnmarshalAny(filterConfig, &httpConnManager)

		if err != nil {
			return nil, err
		}

		routeSpecifier := httpConnManager.GetRouteSpecifier()
		routeConfig := routeSpecifier.(*httpconnmanagerv2.HttpConnectionManager_RouteConfig).RouteConfig

		for _, vHost := range routeConfig.VirtualHosts {
			res = append(res, vHost.Name)
		}

	}

	return res, nil
}
