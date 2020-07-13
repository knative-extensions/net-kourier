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
	"sort"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/google/go-cmp/cmp/cmpopts"

	"knative.dev/net-kourier/pkg/config"

	"knative.dev/networking/pkg/apis/networking/v1alpha1"

	"go.uber.org/zap"

	"gotest.tools/assert"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeleteIngressInfo(t *testing.T) {
	logger := zap.S()
	kubeClient := fake.Clientset{}

	caches, err := NewCaches(logger, &kubeClient, false, nil)
	if err != nil {
		t.Fail()
	}

	// Add info for an ingress
	firstIngressName := "ingress_1"
	firstIngressNamespace := "ingress_1_namespace"
	createTestDataForIngress(
		caches,
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
		caches,
		secondIngressName,
		secondIngressNamespace,
		"cluster_for_ingress_2",
		"route_for_ingress_2",
		"internal_host_for_ingress_2",
		"external_host_for_ingress_2",
		&kubeClient,
	)

	// Delete the first ingress
	_ = caches.DeleteIngressInfo(firstIngressName, firstIngressNamespace, &kubeClient)

	// Check that the listeners only have the virtual hosts of the second
	// ingress.
	// Note: Apart from the vHosts that were added explicitly, there's also
	// the one used to verify the snapshot version.
	vHostsNames, err := getVHostsNames(caches.routeConfig)

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
	logger := zap.S()
	kubeClient := fake.Clientset{}

	caches, err := NewCaches(logger, &kubeClient, false, nil)
	if err != nil {
		t.Fail()
	}

	// Add info for an ingress
	firstIngressName := "ingress_1"
	firstIngressNamespace := "ingress_1_namespace"
	createTestDataForIngress(
		caches,
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

	// This is a temporary workaround. Remove when we delete the route with a
	// randomly generated name in status_vhost.go
	// This deletes the last route of "internal_services" which we know is the
	// one randomly generated and changes when we call caches.ToEnvoySnapshot(),
	// so we do not want to check it, but we want to check everything else which
	// should have not changed.
	vHostsRoutesBefore := routesBeforeDelete["internal_services"].(*v2.RouteConfiguration).VirtualHosts
	routesBeforeDelete["internal_services"].(*v2.RouteConfiguration).VirtualHosts = vHostsRoutesBefore[:len(vHostsRoutesBefore)-1]
	vHostsRoutesAfter := routesAfterDelete["internal_services"].(*v2.RouteConfiguration).VirtualHosts
	routesAfterDelete["internal_services"].(*v2.RouteConfiguration).VirtualHosts = vHostsRoutesAfter[:len(vHostsRoutesAfter)-1]

	assert.DeepEqual(t, clustersBeforeDelete, clustersAfterDelete)
	assert.DeepEqual(t, routesBeforeDelete, routesAfterDelete, cmpopts.IgnoreUnexported(wrappers.BoolValue{}))
	assert.DeepEqual(t, listenersBeforeDelete, listenersAfterDelete, cmpopts.IgnoreUnexported(anypb.Any{}))
}

// Creates an ingress translation and listeners from the given names an
// associates them with the ingress name/namespace received.
func createTestDataForIngress(caches *Caches,
	ingressName string,
	ingressNamespace string,
	clusterName string,
	routeName string,
	internalVHostName string,
	externalVHostName string,
	kubeClient kubeclient.Interface) {

	translatedIngress := translatedIngress{
		ingressName:          ingressName,
		ingressNamespace:     ingressNamespace,
		routes:               []*route.Route{{Name: routeName}},
		clusters:             []*v2.Cluster{{Name: clusterName}},
		externalVirtualHosts: []*route.VirtualHost{{Name: externalVHostName, Domains: []string{externalVHostName}}},
		internalVirtualHosts: []*route.VirtualHost{{Name: internalVHostName, Domains: []string{internalVHostName}}},
	}

	_ = caches.addTranslatedIngress(&v1alpha1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:      ingressName,
			Namespace: ingressNamespace,
		},
	}, &translatedIngress)

	caches.addStatusVirtualHost()
	_ = caches.setListeners(kubeClient)
}

func TestValidateIngress(t *testing.T) {
	logger := zap.S()
	kubeClient := fake.Clientset{}

	caches, err := NewCaches(logger, &kubeClient, false, nil)
	if err != nil {
		t.Fail()
	}

	createTestDataForIngress(
		caches,
		"ingress_1",
		"ingress_1_namespace",
		"cluster_for_ingress_1",
		"route_for_ingress_1",
		"internal_host_for_ingress_1",
		"external_host_for_ingress_1",
		&kubeClient,
	)

	translatedIngress := translatedIngress{
		ingressName:          "ingress_2",
		ingressNamespace:     "ingress_2_namespace",
		routes:               []*route.Route{{Name: "route_for_ingress_2"}},
		clusters:             []*v2.Cluster{{Name: "cluster_for_ingress_2"}},
		externalVirtualHosts: []*route.VirtualHost{{Name: "external_host_for_ingress_2", Domains: []string{"external_host_for_ingress_2"}}},
		//This domain should clash with the cached ingress.
		internalVirtualHosts: []*route.VirtualHost{{Name: "internal_host_for_ingress_2", Domains: []string{"internal_host_for_ingress_1"}}},
	}

	err = caches.validateIngress(&translatedIngress)
	assert.Error(t, err, ErrDomainConflict.Error())
}

func getVHostsNames(routeConfigs []v2.RouteConfiguration) ([]string, error) {
	var res []string

	for _, routeConfig := range routeConfigs {
		for _, vHost := range routeConfig.GetVirtualHosts() {
			res = append(res, vHost.Name)
		}
	}

	return res, nil
}

func TestCacheWithWarmingWithoutIngressesToSync(t *testing.T) {
	logger := zap.S()
	kubeClient := fake.Clientset{}

	var ingressesToSync []*v1alpha1.Ingress
	caches, err := NewCaches(logger, &kubeClient, false, ingressesToSync)
	if err != nil {
		t.Fail()
	}

	// If caches are not synced, let's fail as this should return false.
	if !caches.hasSynced() {
		t.Fail()
	}

	// WaitForSync channel should be closed.
	select {
	case <-caches.WaitForSync():
		return
	default:
		t.Fail()
	}
}

func TestCacheWithWarmingWithIngressesToSync(t *testing.T) {
	logger := zap.S()
	kubeClient := fake.Clientset{}

	ingressesToSync := []*v1alpha1.Ingress{
		{
			TypeMeta:   v1.TypeMeta{},
			ObjectMeta: v1.ObjectMeta{Name: "test1", Namespace: "namespace1"},
			Spec:       v1alpha1.IngressSpec{},
			Status:     v1alpha1.IngressStatus{},
		},
	}
	caches, err := NewCaches(logger, &kubeClient, false, ingressesToSync)
	if err != nil {
		t.Fail()
	}

	// If caches are synced, let's fail, as this should return false.
	if caches.hasSynced() {
		t.Fail()
	}

	// WaitForSync should still be open.
	select {
	case <-caches.WaitForSync():
		t.Fail()
	default:
		// This means the channel has no data and it's still open, this is good, let's continue.
		break
	}

	caches.deleteFromSyncList("test1", "namespace1")

	// If caches are not synced, let's fail as this should return false.
	if !caches.hasSynced() {
		t.Fail()
	}

	// Let's check for the sync channel to be closed.
	select {
	case <-caches.WaitForSync():
		return
	default:
		// Let's fail as the channel has not been closed.
		t.Fail()
	}
}
