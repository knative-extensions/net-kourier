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
	"github.com/google/go-cmp/cmp"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/net-kourier/pkg/envoy"
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.internal.knative.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello-world",
		},
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.IngressRule{
				{
					HTTP: &v1alpha1.HTTPIngressRuleValue{
						Paths: []v1alpha1.HTTPIngressPath{
							{
								Splits: []v1alpha1.IngressBackendSplit{
									{
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
									},
									{
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
									},
								},
							},
						},
					},
				},
			},
		},
		Status: v1alpha1.IngressStatus{},
	}

	kubeClient := fake.NewSimpleClientset()
	ctx := context.Background()
	// Create the Kubernetes services associated to the Knative services that
	// appear in the ingress above
	if err := createServicesWithNames(
		ctx,
		kubeClient,
		[]string{"hello-world-rev1", "hello-world-rev2"},
		"default",
	); err != nil {
		t.Error(err)
	}

	ingressTranslator := NewIngressTranslator(
		kubeClient, newMockedEndpointsLister(), newMockedServiceLister(), &pkgtest.FakeTracker{})

	ingressTranslation, err := ingressTranslator.translateIngress(ctx, &ingress, false)
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, 1, len(ingressTranslation.routes))

	// Check that there are 2 weighted clusters for the route
	envoyRoute := ingressTranslation.routes[0]
	weightedClusters := envoyRoute.GetRoute().GetWeightedClusters().Clusters
	assert.Equal(t, 2, len(weightedClusters))

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
	assert.Equal(
		t,
		true,
		clustersExist([]string{"hello-world-rev1/", "hello-world-rev2/"}, ingressTranslation.clusters),
	)
}

func TestIngressVisibility(t *testing.T) {
	ctx := context.Background()
	kubeClient := fake.NewSimpleClientset()
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

			// Create the Kubernetes services associated to the Knative service that
			// appears in the ingress above
			if err := createServicesWithNames(ctx, kubeClient, []string{ingress.ObjectMeta.Name}, ingress.ObjectMeta.Namespace); err != nil {
				t.Error(err)
			}

			ingressTranslator := NewIngressTranslator(
				kubeClient, newMockedEndpointsLister(), newMockedServiceLister(), &pkgtest.FakeTracker{})

			translatedIngress, err := ingressTranslator.translateIngress(ctx, ingress, false)
			if err != nil {
				t.Error(err)
			}
			var extDomains, intDomains []string
			var extHosts, intHosts []*route.VirtualHost
			extHosts = translatedIngress.externalVirtualHosts
			intHosts = translatedIngress.internalVirtualHosts

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
	tlsCert := "some-cert"
	tlsKey := "tls-key"
	svcNamespace := "default"
	ctx := context.Background()

	ingress := createIngressWithTLS(tlsHosts, tlsSecretName, tlsSecretNamespace, svcNamespace)

	kubeClient := fake.NewSimpleClientset()

	// Create the Kubernetes services associated to the Knative service that
	// appears in the ingress above
	if err := createServicesWithNames(ctx, kubeClient, []string{ingress.ObjectMeta.Name}, svcNamespace); err != nil {
		t.Error(err)
	}

	// Create secret with TLS data
	err := createSecret(ctx, kubeClient, tlsSecretNamespace, tlsSecretName, tlsCert, tlsKey)
	if err != nil {
		t.Error(err)
	}

	ingressTranslator := NewIngressTranslator(
		kubeClient, newMockedEndpointsLister(), newMockedServiceLister(), &pkgtest.FakeTracker{})

	translatedIngress, err := ingressTranslator.translateIngress(ctx, ingress, false)
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, 1, len(translatedIngress.sniMatches))
	assert.DeepEqual(
		t,
		envoy.NewSNIMatch(tlsHosts, tlsCert, tlsKey),
		*translatedIngress.sniMatches[0],
		cmp.AllowUnexported(envoy.SNIMatch{}),
	)
}

func TestReturnsErrorWhenTLSSecretDoesNotExist(t *testing.T) {
	tlsSecretName := "tls-secret"
	tlsSecretNamespace := "default"
	tlsHosts := []string{"hello-world.example.com"}
	svcNamespace := "default"

	ingress := createIngressWithTLS(tlsHosts, tlsSecretName, tlsSecretNamespace, svcNamespace)

	kubeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	// Create the Kubernetes services associated to the Knative service that
	// appears in the ingress above
	if err := createServicesWithNames(ctx, kubeClient, []string{ingress.ObjectMeta.Name}, "default"); err != nil {
		t.Error(err)
	}

	ingressTranslator := NewIngressTranslator(
		kubeClient, newMockedEndpointsLister(), newMockedServiceLister(), &pkgtest.FakeTracker{})

	_, err := ingressTranslator.translateIngress(ctx, ingress, false)

	assert.Error(t, err, fmt.Sprintf("failed to get sniMatch: secrets \"%s\" not found", tlsSecretName))
}

func newMockedEndpointsLister() corev1listers.EndpointsLister {
	return new(endpointsLister)
}

type endpointsLister struct{}

func (endpointsLister *endpointsLister) List(selector labels.Selector) ([]*corev1.Endpoints, error) {
	return []*corev1.Endpoints{{}}, nil
}

func (endpointsLister *endpointsLister) Endpoints(namespace string) corev1listers.EndpointsNamespaceLister {
	return new(endpoints)
}

type endpoints struct{}

func (endpoints *endpoints) List(selector labels.Selector) ([]*corev1.Endpoints, error) {
	return []*corev1.Endpoints{{}}, nil
}

func (endpoints *endpoints) Get(name string) (*corev1.Endpoints, error) {
	return &corev1.Endpoints{}, nil
}

func newMockedServiceLister() corev1listers.ServiceLister {
	return new(serviceLister)
}

type serviceLister struct{}

func (endpointsLister *serviceLister) List(selector labels.Selector) ([]*corev1.Service, error) {
	return []*corev1.Service{{}}, nil
}

func (endpointsLister *serviceLister) Services(namespace string) corev1listers.ServiceNamespaceLister {
	return new(service)
}

type service struct{}

func (endpoints *service) List(selector labels.Selector) ([]*corev1.Service, error) {
	return []*corev1.Service{{}}, nil
}

func (endpoints *service) Get(name string) (*corev1.Service, error) {
	return &corev1.Service{}, nil
}

func createIngress(name string, hosts []string, visibility v1alpha1.IngressVisibility) *v1alpha1.Ingress {
	return &v1alpha1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.internal.knative.dev/v1alpha1",
		},
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
		Status: v1alpha1.IngressStatus{},
	}
}

func createIngressWithTLS(hosts []string, secretName string, secretNamespace string, svcNamespace string) *v1alpha1.Ingress {
	return &v1alpha1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.internal.knative.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "hello-world",
		},
		Spec: v1alpha1.IngressSpec{
			TLS: []v1alpha1.IngressTLS{{
				Hosts:           hosts,
				SecretName:      secretName,
				SecretNamespace: secretNamespace,
			}},
			Rules: []v1alpha1.IngressRule{
				{
					Visibility: v1alpha1.IngressVisibilityExternalIP,
					HTTP: &v1alpha1.HTTPIngressRuleValue{
						Paths: []v1alpha1.HTTPIngressPath{
							{
								Splits: []v1alpha1.IngressBackendSplit{
									{
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
									},
								},
							},
						},
					},
				},
			},
		},
		Status: v1alpha1.IngressStatus{},
	}

}

func createSecret(ctx context.Context, kubeClient kubernetes.Interface, secretNamespace string, secretName string, cert string, key string) error {
	tlsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Data: map[string][]byte{
			certFieldInSecret: []byte(cert),
			keyFieldInSecret:  []byte(key),
		},
	}

	_, err := kubeClient.CoreV1().Secrets(secretNamespace).Create(ctx, &tlsSecret, metav1.CreateOptions{})
	return err
}

func createServicesWithNames(ctx context.Context, kubeclient kubernetes.Interface, names []string, namespace string) error {
	for _, serviceName := range names {
		service := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: serviceName,
			},
		}

		_, err := kubeclient.CoreV1().Services(namespace).Create(ctx, &service, metav1.CreateOptions{})

		if err != nil {
			return err
		}
	}

	return nil
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
