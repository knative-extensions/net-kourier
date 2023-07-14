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
	"strconv"
	"testing"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	http_connection_managerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	rconfig "knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/certificates"
	netconfig "knative.dev/networking/pkg/config"
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
		"external_tls_host_for_ingress_1",
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
		"external_tls_host_for_ingress_2",
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
		"external_tls_host_for_ingress_2",
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
		"external_tls_host_for_ingress_1",
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

func TestTLSListenerWithEnvCertsSecret(t *testing.T) {
	t.Setenv(envCertsSecretNamespace, "certns")
	t.Setenv(envCertsSecretName, "secretname")

	kubeClient := fake.Clientset{}
	ctx := context.Background()

	caches, err := NewCaches(ctx, &kubeClient, false)
	assert.NilError(t, err)

	fooSNIMatch := &envoy.SNIMatch{
		Hosts:            []string{"foo.example.com"},
		CertSource:       types.NamespacedName{Namespace: "secretns", Name: "secretname1"},
		CertificateChain: []byte("cert1"),
		PrivateKey:       []byte("privateKey1"),
	}
	barSNIMatch := &envoy.SNIMatch{
		Hosts:            []string{"bar.example.com"},
		CertSource:       types.NamespacedName{Namespace: "secretns", Name: "secretname2"},
		CertificateChain: []byte("cert2"),
		PrivateKey:       []byte("privateKey2"),
	}

	t.Run("without SNI matches", func(t *testing.T) {
		translatedIngress := &translatedIngress{
			sniMatches: nil,
		}
		err := caches.addTranslatedIngress(translatedIngress)
		assert.NilError(t, err)

		snapshot, err := caches.ToEnvoySnapshot(ctx)
		assert.NilError(t, err)

		tlsListener := snapshot.GetResources(resource.ListenerType)[envoy.CreateListenerName(config.HTTPSPortExternal)].(*listener.Listener)
		filterChains := tlsListener.FilterChains
		assert.Assert(t, len(filterChains) == 1)
	})

	t.Run("with a single SNI match", func(t *testing.T) {
		translatedIngress := &translatedIngress{
			sniMatches: []*envoy.SNIMatch{fooSNIMatch},
		}
		err := caches.addTranslatedIngress(translatedIngress)
		assert.NilError(t, err)

		snapshot, err := caches.ToEnvoySnapshot(ctx)
		assert.NilError(t, err)

		tlsListener := snapshot.GetResources(resource.ListenerType)[envoy.CreateListenerName(config.HTTPSPortExternal)].(*listener.Listener)
		filterChains := tlsListener.FilterChains
		assert.Assert(t, len(filterChains) == 2)

		filterChainsByServerName := map[string]*listener.FilterChain{}
		for _, filterChain := range filterChains {
			var serverName string
			if filterChain.FilterChainMatch != nil {
				serverName = filterChain.FilterChainMatch.ServerNames[0]
			}
			filterChainsByServerName[serverName] = filterChain
		}

		assert.Check(t, filterChainsByServerName["foo.example.com"] != nil)
		assert.Check(t, filterChainsByServerName[""] != nil) // filter chain without server name, "default" one
	})

	t.Run("with multiple SNI matches", func(t *testing.T) {
		translatedIngress := &translatedIngress{
			sniMatches: []*envoy.SNIMatch{fooSNIMatch, barSNIMatch},
		}
		err := caches.addTranslatedIngress(translatedIngress)
		assert.NilError(t, err)

		snapshot, err := caches.ToEnvoySnapshot(ctx)
		assert.NilError(t, err)

		tlsListener := snapshot.GetResources(resource.ListenerType)[envoy.CreateListenerName(config.HTTPSPortExternal)].(*listener.Listener)
		filterChains := tlsListener.FilterChains
		assert.Assert(t, len(filterChains) == 3)

		filterChainsByServerName := map[string]*listener.FilterChain{}
		for _, filterChain := range filterChains {
			var serverName string
			if filterChain.FilterChainMatch != nil {
				serverName = filterChain.FilterChainMatch.ServerNames[0]
			}
			filterChainsByServerName[serverName] = filterChain
		}

		assert.Check(t, filterChainsByServerName["foo.example.com"] != nil)
		assert.Check(t, filterChainsByServerName["bar.example.com"] != nil)
		assert.Check(t, filterChainsByServerName[""] != nil) // filter chain without server name, "default" one
	})
}

// TestTLSListenerWithInternalCertSecret verfies that
// filter is added when secret name is specified by cluster-cert-secret.
func TestTLSListenerWithInternalCertSecret(t *testing.T) {
	testConfig := &rconfig.Config{
		Network: &netconfig.Config{},
		Kourier: &config.Kourier{
			ClusterCertSecret:   "test-ca",
			EnableProxyProtocol: true,
		},
	}

	internalSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ca",
		},
		Data: map[string][]byte{
			certificates.CaCertName: cert,
		},
	}

	kubeClient := fake.Clientset{}
	cfg := testConfig.DeepCopy()
	ctx := (&testConfigStore{config: cfg}).ToContext(context.Background())

	_, err := kubeClient.CoreV1().Secrets("knative-serving").Create(ctx, internalSecret, metav1.CreateOptions{})
	assert.NilError(t, err)

	caches, err := NewCaches(ctx, &kubeClient, false)
	assert.NilError(t, err)

	t.Run("without SNI matches", func(t *testing.T) {
		translatedIngress := &translatedIngress{
			sniMatches: nil,
		}
		err := caches.addTranslatedIngress(translatedIngress)
		assert.NilError(t, err)

		snapshot, err := caches.ToEnvoySnapshot(ctx)
		assert.NilError(t, err)

		tlsListener := snapshot.GetResources(resource.ListenerType)[envoy.CreateListenerName(config.HTTPSPortInternal)].(*listener.Listener)
		assert.Assert(t, len(tlsListener.ListenerFilters) == 1)
		assert.Assert(t, (tlsListener.ListenerFilters[0]).Name == wellknown.ProxyProtocol)
	})
}

// TestListenersAndClustersWithTracing verifies that when we enable tracing
// a cluster is added for the tracing backend, and tracing configuration is added to all listeners.
func TestListenersAndClustersWithTracing(t *testing.T) {
	tracingCollectorHost := "jaeger.default.svc.cluster.local"
	tracingCollectorPort := 9411
	tracingCollectorEndpoint := "/api/v2/spans"

	kourierCfg, err := config.NewConfigFromMap(map[string]string{
		config.TracingEnabled:           "true",
		config.TracingCollectorHost:     tracingCollectorHost,
		config.TracingCollectorPort:     strconv.Itoa(tracingCollectorPort),
		config.TracingCollectorEndpoint: tracingCollectorEndpoint,
	})
	assert.NilError(t, err)

	testConfig := &rconfig.Config{
		Kourier: kourierCfg,
	}

	kubeClient := fake.Clientset{}
	cfg := testConfig.DeepCopy()
	ctx := (&testConfigStore{config: cfg}).ToContext(context.Background())

	caches, err := NewCaches(ctx, &kubeClient, false)
	assert.NilError(t, err)

	t.Run("check a tracing cluster exist, and tracing is configured on listeners", func(t *testing.T) {
		translatedIngress := &translatedIngress{}
		err := caches.addTranslatedIngress(translatedIngress)
		assert.NilError(t, err)

		snapshot, err := caches.ToEnvoySnapshot(ctx)
		assert.NilError(t, err)

		tracingCollectorCluster := snapshot.GetResources(resource.ClusterType)["tracing-collector"].(*v3.Cluster)

		expectedTracingCollectorCluster := &v3.Cluster{
			Name:                 "tracing-collector",
			ClusterDiscoveryType: &v3.Cluster_Type{Type: v3.Cluster_STRICT_DNS},
			LoadAssignment: &endpoint.ClusterLoadAssignment{
				ClusterName: "tracing-collector",
				Endpoints: []*endpoint.LocalityLbEndpoints{{
					LbEndpoints: []*endpoint.LbEndpoint{{
						HostIdentifier: &endpoint.LbEndpoint_Endpoint{
							Endpoint: &endpoint.Endpoint{
								Address: &core.Address{
									Address: &core.Address_SocketAddress{
										SocketAddress: &core.SocketAddress{
											Protocol: core.SocketAddress_TCP,
											Address:  tracingCollectorHost,
											PortSpecifier: &core.SocketAddress_PortValue{
												PortValue: uint32(tracingCollectorPort),
											},
											Ipv4Compat: true,
										},
									},
								},
							},
						},
					}},
				}},
			},
		}

		assert.DeepEqual(t, expectedTracingCollectorCluster, tracingCollectorCluster,
			cmpopts.IgnoreUnexported(
				v3.Cluster{}, endpoint.ClusterLoadAssignment{}, endpoint.LocalityLbEndpoints{}, endpoint.LbEndpoint{},
				endpoint.Endpoint{}, core.Address{}, core.SocketAddress{},
			),
		)

		listenersPorts := []uint32{
			config.HTTPPortExternal, config.HTTPPortInternal, config.HTTPPortProb,
		}

		for _, listenerPort := range listenersPorts {
			currentListener := snapshot.GetResources(resource.ListenerType)[envoy.CreateListenerName(listenerPort)].(*listener.Listener)

			expectedTracingConfig := &tracev3.ZipkinConfig{
				CollectorCluster:         "tracing-collector",
				CollectorEndpoint:        tracingCollectorEndpoint,
				SharedSpanContext:        wrapperspb.Bool(false),
				CollectorEndpointVersion: tracev3.ZipkinConfig_HTTP_JSON,
			}

			httpConnectionManagerFilter := currentListener.FilterChains[0].Filters[0]
			assert.Equal(t, wellknown.HTTPConnectionManager, httpConnectionManagerFilter.Name)

			httpConnectionManagerConfig := &http_connection_managerv3.HttpConnectionManager{}
			err = anypb.UnmarshalTo(httpConnectionManagerFilter.GetTypedConfig(), httpConnectionManagerConfig, proto.UnmarshalOptions{})
			assert.NilError(t, err)

			tracingConfig := &tracev3.ZipkinConfig{}
			err = anypb.UnmarshalTo(httpConnectionManagerConfig.Tracing.Provider.GetTypedConfig(), tracingConfig, proto.UnmarshalOptions{})
			assert.NilError(t, err)
			assert.DeepEqual(t, expectedTracingConfig, tracingConfig, cmpopts.IgnoreUnexported(tracev3.ZipkinConfig{}, wrapperspb.BoolValue{}))
		}
	})
}

// Creates an ingress translation and listeners from the given names an
// associates them with the ingress name/namespace received.
func createTestDataForIngress(
	caches *Caches,
	ingressName string,
	ingressNamespace string,
	clusterName string,
	internalVHostName string,
	externalVHostName string,
	externalTLSVHostName string) {

	translatedIngress := &translatedIngress{
		name: types.NamespacedName{
			Namespace: ingressNamespace,
			Name:      ingressName,
		},
		clusters:                []*v3.Cluster{{Name: clusterName}},
		externalVirtualHosts:    []*route.VirtualHost{{Name: externalVHostName, Domains: []string{externalVHostName}}},
		externalTLSVirtualHosts: []*route.VirtualHost{{Name: externalTLSVHostName, Domains: []string{externalTLSVHostName}}},
		internalVirtualHosts:    []*route.VirtualHost{{Name: internalVHostName, Domains: []string{internalVHostName}}},
		sniMatches: []*envoy.SNIMatch{{
			Hosts:            []string{"foo.example.com"},
			CertSource:       types.NamespacedName{Namespace: "secretns", Name: "secretname"},
			CertificateChain: cert,
			PrivateKey:       privateKey}},
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
		"external_tls_host_for_ingress_1",
	)

	translatedIngress := translatedIngress{
		name: types.NamespacedName{
			Namespace: "ingress_2_namespace",
			Name:      "ingress_2",
		},
		clusters:                []*v3.Cluster{{Name: "cluster_for_ingress_2"}},
		externalVirtualHosts:    []*route.VirtualHost{{Name: "external_host_for_ingress_2", Domains: []string{"external_host_for_ingress_2"}}},
		externalTLSVirtualHosts: []*route.VirtualHost{{Name: "external_tls_host_for_ingress_2", Domains: []string{"external__tlshost_for_ingress_2"}}},
		//This domain should clash with the cached ingress.
		internalVirtualHosts: []*route.VirtualHost{{Name: "internal_host_for_ingress_2", Domains: []string{"internal_host_for_ingress_1"}}},
		sniMatches: []*envoy.SNIMatch{{
			Hosts:            []string{"foo.example.com"},
			CertSource:       types.NamespacedName{Namespace: "secretns", Name: "secretname"},
			CertificateChain: cert,
			PrivateKey:       privateKey}},
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

func TestAddIsolatedIngress(t *testing.T) {
	kubeClient := fake.Clientset{}
	ctx := context.Background()

	caches, err := NewCaches(ctx, &kubeClient, false)
	assert.NilError(t, err)

	translatedIngress := translatedIngress{
		name: types.NamespacedName{
			Namespace: "ingress_2_namespace",
			Name:      "ingress_2",
		},
		listenerPort:         "12158",
		internalVirtualHosts: []*route.VirtualHost{{Name: "internal_host_for_ingress_2", Domains: []string{"internal_host_for_ingress_1"}}},
	}

	err = caches.addTranslatedIngress(&translatedIngress)
	assert.NilError(t, err)

	snapshot, err := caches.ToEnvoySnapshot(ctx)
	assert.NilError(t, err)

	ls := snapshot.GetResources(resource.ListenerType)
	assert.Assert(t, ls != nil)

	l, ok := ls["listener_12158"].(*listener.Listener)
	assert.Assert(t, ok)
	assert.Equal(t, l.GetAddress().GetSocketAddress().GetPortValue(), uint32(12158))
}
