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
	"fmt"
	"sort"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	pkgtest "knative.dev/pkg/reconciler/testing"
)

// Tests that when there is a traffic split defined in the ingress:
// - Creates a route with weighted clusters defined.
// - There's one weighted cluster for each traffic split and each contains:
// 		- The traffic percentage.
//		- The headers to add (namespace and revision name).
//		- The cluster name, based on the revision name plus the path.
// - The weighted clusters exists also in the clusters cache with the same
//   name.
//
// Note: for now, the name of the cluster is the name of the revision plus the
// path. That might change in the future.
func TestTrafficSplits(t *testing.T) {
	ingress := v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello-world",
		},
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.IngressRule{{
				HTTP: &v1alpha1.HTTPIngressRuleValue{
					Paths: []v1alpha1.HTTPIngressPath{{
						Splits: []v1alpha1.IngressBackendSplit{{
							IngressBackend: v1alpha1.IngressBackend{
								ServiceNamespace: "default",
								ServiceName:      "hello-world-rev1",
								ServicePort: intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 80,
								},
							},
							Percent: 60,
							AppendHeaders: map[string]string{
								"Knative-Serving-Namespace": "default",
								"Knative-Serving-Revision":  "hello-world-rev1",
							},
						}, {
							IngressBackend: v1alpha1.IngressBackend{
								ServiceNamespace: "default",
								ServiceName:      "hello-world-rev2",
								ServicePort: intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 80,
								},
							},
							Percent: 40,
							AppendHeaders: map[string]string{
								"Knative-Serving-Namespace": "default",
								"Knative-Serving-Revision":  "hello-world-rev2",
							},
						}},
					}},
				},
			}},
		},
	}
	ctx := context.Background()
	ingressTranslator := NewIngressTranslator(
		mockedSecretGetter, mockedEndpointsGetter, mockedServiceGetter, &pkgtest.FakeTracker{})

	ingressTranslation, err := ingressTranslator.translateIngress(ctx, &ingress, false)
	assert.NilError(t, err)

	vHosts := ingressTranslation.internalVirtualHosts
	assert.Assert(t, is.Len(vHosts, 1))

	routes := vHosts[0].GetRoutes()
	assert.Assert(t, is.Len(routes, 1))

	// Check that there are 2 weighted clusters for the route
	weightedClusters := routes[0].GetRoute().GetWeightedClusters().GetClusters()
	assert.Assert(t, is.Len(weightedClusters, 2))

	// Check the first weighted cluster
	assertWeightedClusterCorrect(
		t,
		weightedClusters[0],
		"hello-world-rev1/",
		uint32(60),
		map[string]string{
			"Knative-Serving-Namespace": "default",
			"Knative-Serving-Revision":  "hello-world-rev1",
		},
	)

	// Check the second weighted cluster
	assertWeightedClusterCorrect(
		t,
		weightedClusters[1],
		"hello-world-rev2/",
		uint32(40),
		map[string]string{
			"Knative-Serving-Namespace": "default",
			"Knative-Serving-Revision":  "hello-world-rev2",
		},
	)

	// Check the clusters cache
	assert.Assert(t,
		clustersExist([]string{"hello-world-rev1/", "hello-world-rev2/"}, ingressTranslation.clusters))
}

func TestExternalNameService(t *testing.T) {
	ctx := context.Background()

	ingress := createIngress("test", []string{"foo", "bar"}, v1alpha1.IngressVisibilityExternalIP)
	svc := &corev1.Service{
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "example.com",
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	ingressTranslator := NewIngressTranslator(
		mockedSecretGetter,
		mockedEndpointsGetter,
		func(ns, name string) (*corev1.Service, error) {
			return svc, nil
		},
		&pkgtest.FakeTracker{})

	translatedIngress, err := ingressTranslator.translateIngress(ctx, ingress, false)
	assert.NilError(t, err)

	clusters := translatedIngress.clusters
	assert.Assert(t, is.Len(clusters, 1))

	cluster := clusters[0]
	assert.Assert(t, is.Nil(cluster.Http2ProtocolOptions))
	assert.Equal(t, cluster.GetType(), v2.Cluster_LOGICAL_DNS)

	localityEps := cluster.GetLoadAssignment().GetEndpoints()
	assert.Assert(t, is.Len(localityEps, 1))

	eps := localityEps[0].LbEndpoints
	assert.Assert(t, is.Len(eps, 1))

	ep := eps[0]
	assert.Equal(t, ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress(), "example.com")
}

func TestIngressVisibility(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		hosts      []string
		extDomains []string
		intDomains []string
		visibility v1alpha1.IngressVisibility
	}{{
		name:       "external visibility",
		hosts:      []string{"hello.default.example.com"},
		extDomains: []string{"hello.default.example.com", "hello.default.example.com:*"},
		// External should also be accessible internally
		intDomains: []string{"hello.default.example.com", "hello.default.example.com:*"},
		visibility: v1alpha1.IngressVisibilityExternalIP,
	}, {
		name:       "cluster local visibility",
		hosts:      []string{"hello.default", "hello.default.svc", "hello.default.svc.cluster.local"},
		intDomains: []string{"hello.default", "hello.default:*", "hello.default.svc", "hello.default.svc:*", "hello.default.svc.cluster.local", "hello.default.svc.cluster.local:*"},
		visibility: v1alpha1.IngressVisibilityClusterLocal,
	}}

	for num, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ingress := createIngress(fmt.Sprintf("hello-%d", num), test.hosts, test.visibility)
			ingressTranslator := NewIngressTranslator(
				mockedSecretGetter, mockedEndpointsGetter, mockedServiceGetter, &pkgtest.FakeTracker{})

			translatedIngress, err := ingressTranslator.translateIngress(ctx, ingress, false)
			assert.NilError(t, err)

			extHosts := translatedIngress.externalVirtualHosts
			intHosts := translatedIngress.internalVirtualHosts

			var extDomains, intDomains []string
			for _, v := range extHosts {
				extDomains = append(extDomains, v.Domains...)
			}
			for _, v := range intHosts {
				intDomains = append(intDomains, v.Domains...)
			}

			sort.Strings(extDomains)
			sort.Strings(intDomains)
			sort.Strings(test.extDomains)
			sort.Strings(test.intDomains)

			assert.DeepEqual(t, extDomains, test.extDomains)
			assert.DeepEqual(t, intDomains, test.intDomains)
		})
	}
}

func TestIngressWithTLS(t *testing.T) {
	// TLS data for the test
	tlsSecretName := "tls-secret"
	tlsSecretNamespace := "default"
	tlsHosts := []string{"hello-world.example.com"}
	tlsCert := []byte("some-cert")
	tlsKey := []byte("tls-key")
	svcNamespace := "default"
	ctx := context.Background()

	ingress := createIngressWithTLS(tlsHosts, tlsSecretName, tlsSecretNamespace, svcNamespace)

	// Create secret with TLS data
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tlsSecretNamespace,
			Name:      tlsSecretName,
		},
		Data: map[string][]byte{
			certFieldInSecret: tlsCert,
			keyFieldInSecret:  tlsKey,
		},
	}
	secretRef := types.NamespacedName{
		Namespace: tlsSecretNamespace,
		Name:      tlsSecretName,
	}

	ingressTranslator := NewIngressTranslator(
		func(ns, name string) (*corev1.Secret, error) {
			return tlsSecret, nil
		},
		mockedEndpointsGetter,
		mockedServiceGetter,
		&pkgtest.FakeTracker{})

	translatedIngress, err := ingressTranslator.translateIngress(ctx, ingress, false)
	assert.NilError(t, err)

	assert.Assert(t, is.Len(translatedIngress.sniMatches, 1))
	assert.DeepEqual(
		t,
		&envoy.SNIMatch{
			Hosts:            tlsHosts,
			CertSource:       secretRef,
			CertificateChain: tlsCert,
			PrivateKey:       tlsKey,
		},
		translatedIngress.sniMatches[0])
}

func TestReturnsErrorWhenTLSSecretDoesNotExist(t *testing.T) {
	tlsSecretName := "tls-secret"
	tlsSecretNamespace := "default"
	tlsHosts := []string{"hello-world.example.com"}
	svcNamespace := "default"

	ingress := createIngressWithTLS(tlsHosts, tlsSecretName, tlsSecretNamespace, svcNamespace)
	ctx := context.Background()

	ingressTranslator := NewIngressTranslator(
		func(ns, name string) (*corev1.Secret, error) {
			return nil, fmt.Errorf("secrets %q not found", name)
		},
		mockedEndpointsGetter,
		mockedServiceGetter,
		&pkgtest.FakeTracker{})

	_, err := ingressTranslator.translateIngress(ctx, ingress, false)
	assert.Error(t, err, fmt.Sprintf("failed to fetch secret: secrets %q not found", tlsSecretName))
}

var mockedSecretGetter = func(ns, name string) (*corev1.Secret, error) {
	return &corev1.Secret{}, nil
}

var mockedEndpointsGetter = func(ns, name string) (*corev1.Endpoints, error) {
	return &corev1.Endpoints{}, nil
}

var mockedServiceGetter = func(ns, name string) (*corev1.Service, error) {
	return &corev1.Service{}, nil
}

func createIngress(name string, hosts []string, visibility v1alpha1.IngressVisibility) *v1alpha1.Ingress {
	return &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.IngressRule{{
				Hosts:      hosts,
				Visibility: visibility,
				HTTP: &v1alpha1.HTTPIngressRuleValue{
					Paths: []v1alpha1.HTTPIngressPath{{
						Splits: []v1alpha1.IngressBackendSplit{{
							IngressBackend: v1alpha1.IngressBackend{
								ServiceName: name,
								ServicePort: intstr.FromInt(80),
							},
						}},
					}},
				},
			}},
		},
	}
}

func createIngressWithTLS(hosts []string, secretName string, secretNamespace string, svcNamespace string) *v1alpha1.Ingress {
	return &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello-world",
		},
		Spec: v1alpha1.IngressSpec{
			TLS: []v1alpha1.IngressTLS{{
				Hosts:           hosts,
				SecretName:      secretName,
				SecretNamespace: secretNamespace,
			}},
			Rules: []v1alpha1.IngressRule{{
				Visibility: v1alpha1.IngressVisibilityExternalIP,
				HTTP: &v1alpha1.HTTPIngressRuleValue{
					Paths: []v1alpha1.HTTPIngressPath{{
						Splits: []v1alpha1.IngressBackendSplit{{
							IngressBackend: v1alpha1.IngressBackend{
								ServiceNamespace: svcNamespace,
								ServiceName:      "hello-world",
								ServicePort: intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 80,
								},
							},
							AppendHeaders: map[string]string{
								"Knative-Serving-Namespace": svcNamespace,
								"Knative-Serving-Revision":  "hello-world",
							},
						}},
					}},
				},
			}},
		},
	}

}

func clustersExist(names []string, clusters []*v2.Cluster) bool {
	// Create map that contains names that are present
	present := make(map[string]bool)
	for _, cacheCluster := range clusters {
		present[cacheCluster.Name] = true
	}

	// Verify if the received names are present
	for _, name := range names {
		if !present[name] {
			return false
		}
	}

	return true
}

// Checks whether the weightedCluster received has the received name, traffic
// percentage and headers to add
func assertWeightedClusterCorrect(t *testing.T,
	weightedCluster *route.WeightedCluster_ClusterWeight,
	name string,
	trafficPerc uint32,
	headersToAdd map[string]string) {

	assert.Equal(t, name, weightedCluster.Name)

	assert.Equal(t, trafficPerc, weightedCluster.Weight.Value)

	// Collect headers for easier comparison
	clusterHeaders := make(map[string]string)
	for _, header := range weightedCluster.RequestHeadersToAdd {
		clusterHeaders[header.Header.Key] = header.Header.Value
	}
	assert.DeepEqual(t, clusterHeaders, headersToAdd)
}

func TestDomainsForRule(t *testing.T) {
	domains := domainsForRule(v1alpha1.IngressRule{
		Hosts: []string{
			"helloworld-go.default.svc.cluster.local",
			"helloworld-go.default.svc",
			"helloworld-go.default",
		},
	})

	expected := []string{
		"helloworld-go.default",
		"helloworld-go.default:*",
		"helloworld-go.default.svc",
		"helloworld-go.default.svc:*",
		"helloworld-go.default.svc.cluster.local",
		"helloworld-go.default.svc.cluster.local:*",
	}
	sort.Strings(domains)
	sort.Strings(expected)
	assert.DeepEqual(t, domains, expected)
}
