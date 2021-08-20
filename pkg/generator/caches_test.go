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
	"sort"
	"testing"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/testing/protocmp"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/net-kourier/pkg/config"
)

func TestDeleteIngressInfo(t *testing.T) {
	kubeClient := fake.Clientset{}
	ctx := context.Background()

	caches, err := NewCaches(ctx, &kubeClient, false)
	assert.NilError(t, err)

	// Add info for an ingress
	firstIngressName := "ingress_1"
	firstIngressNamespace := "ingress_1_namespace"
	createTestDataForIngress(
		caches,
		firstIngressName,
		firstIngressNamespace,
		"cluster_for_ingress_1",
		"internal_host_for_ingress_1",
		"external_host_for_ingress_1",
	)

	// Add info for a different ingress
	secondIngressName := "ingress_2"
	secondIngressNamespace := "ingress_2_namespace"
	createTestDataForIngress(
		caches,
		secondIngressName,
		secondIngressNamespace,
		"cluster_for_ingress_2",
		"internal_host_for_ingress_2",
		"external_host_for_ingress_2",
	)

	// Delete the first ingress
	caches.DeleteIngressInfo(ctx, firstIngressName, firstIngressNamespace)

	snapshot, err := caches.ToEnvoySnapshot(ctx)
	assert.NilError(t, err)

	routeConfigsR := snapshot.GetResources(resource.RouteType)
	routeConfigs := make([]*route.RouteConfiguration, len(routeConfigsR))
	for _, r := range routeConfigsR {
		routeConfigs = append(routeConfigs, r.(*route.RouteConfiguration))
	}

	// Check that the listeners only have the virtual hosts of the second
	// ingress.
	// Note: Apart from the vHosts that were added explicitly, there's also
	// the one used to verify the snapshot version.
	vHostsNames := getVHostsNames(routeConfigs)

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
	kubeClient := fake.Clientset{}
	ctx := context.Background()

	caches, err := NewCaches(ctx, &kubeClient, false)
	assert.NilError(t, err)

	// Add info for an ingress
	firstIngressName := "ingress_1"
	firstIngressNamespace := "ingress_1_namespace"
	createTestDataForIngress(
		caches,
		firstIngressName,
		firstIngressNamespace,
		"cluster_for_ingress_1",
		"internal_host_for_ingress_1",
		"external_host_for_ingress_1",
	)

	snapshotBeforeDelete, err := caches.ToEnvoySnapshot(ctx)
	assert.NilError(t, err)

	clustersBeforeDelete := snapshotBeforeDelete.GetResources(resource.ClusterType)
	routesBeforeDelete := snapshotBeforeDelete.GetResources(resource.RouteType)
	listenersBeforeDelete := snapshotBeforeDelete.GetResources(resource.ListenerType)

	err = caches.DeleteIngressInfo(ctx, "non_existing_name", "non_existing_namespace")
	assert.NilError(t, err)

	snapshotAfterDelete, err := caches.ToEnvoySnapshot(ctx)
	assert.NilError(t, err)

	clustersAfterDelete := snapshotAfterDelete.GetResources(resource.ClusterType)
	routesAfterDelete := snapshotAfterDelete.GetResources(resource.RouteType)
	listenersAfterDelete := snapshotAfterDelete.GetResources(resource.ListenerType)

	// This is a temporary workaround. Remove when we delete the route with a
	// randomly generated name in status_vhost.go
	// This deletes the last route of "internal_services" which we know is the
	// one randomly generated and changes when we call caches.ToEnvoySnapshot(),
	// so we do not want to check it, but we want to check everything else which
	// should have not changed.
	vHostsRoutesBefore := routesBeforeDelete["internal_services"].(*route.RouteConfiguration).VirtualHosts
	routesBeforeDelete["internal_services"].(*route.RouteConfiguration).VirtualHosts = vHostsRoutesBefore[:len(vHostsRoutesBefore)-1]
	vHostsRoutesAfter := routesAfterDelete["internal_services"].(*route.RouteConfiguration).VirtualHosts
	routesAfterDelete["internal_services"].(*route.RouteConfiguration).VirtualHosts = vHostsRoutesAfter[:len(vHostsRoutesAfter)-1]

	assert.DeepEqual(t, clustersBeforeDelete, clustersAfterDelete, protocmp.Transform())
	assert.DeepEqual(t, routesBeforeDelete, routesAfterDelete, protocmp.Transform())
	assert.DeepEqual(t, listenersBeforeDelete, listenersAfterDelete, protocmp.Transform())
}

// Creates an ingress translation and listeners from the given names an
// associates them with the ingress name/namespace received.
func createTestDataForIngress(
	caches *Caches,
	ingressName string,
	ingressNamespace string,
	clusterName string,
	internalVHostName string,
	externalVHostName string) {

	translatedIngress := &translatedIngress{
		name: types.NamespacedName{
			Namespace: ingressNamespace,
			Name:      ingressName,
		},
		clusters:             []*v3.Cluster{{Name: clusterName}},
		externalVirtualHosts: []*route.VirtualHost{{Name: externalVHostName, Domains: []string{externalVHostName}}},
		internalVirtualHosts: []*route.VirtualHost{{Name: internalVHostName, Domains: []string{internalVHostName}}},
	}

	caches.addTranslatedIngress(translatedIngress)
}

func TestValidateIngress(t *testing.T) {
	kubeClient := fake.Clientset{}
	ctx := context.Background()

	caches, err := NewCaches(ctx, &kubeClient, false)
	assert.NilError(t, err)

	createTestDataForIngress(
		caches,
		"ingress_1",
		"ingress_1_namespace",
		"cluster_for_ingress_1",
		"internal_host_for_ingress_1",
		"external_host_for_ingress_1",
	)

	translatedIngress := translatedIngress{
		name: types.NamespacedName{
			Namespace: "ingress_2_namespace",
			Name:      "ingress_2",
		},
		clusters:             []*v3.Cluster{{Name: "cluster_for_ingress_2"}},
		externalVirtualHosts: []*route.VirtualHost{{Name: "external_host_for_ingress_2", Domains: []string{"external_host_for_ingress_2"}}},
		//This domain should clash with the cached ingress.
		internalVirtualHosts: []*route.VirtualHost{{Name: "internal_host_for_ingress_2", Domains: []string{"internal_host_for_ingress_1"}}},
	}

	err = caches.validateIngress(&translatedIngress)
	assert.Error(t, err, ErrDomainConflict.Error())
}

func getVHostsNames(routeConfigs []*route.RouteConfiguration) []string {
	var res []string

	for _, routeConfig := range routeConfigs {
		for _, vHost := range routeConfig.GetVirtualHosts() {
			res = append(res, vHost.Name)
		}
	}

	return res
}
