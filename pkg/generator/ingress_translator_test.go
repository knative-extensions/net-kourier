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
	"kourier/pkg/envoy"
	"testing"

	"github.com/google/go-cmp/cmp"

	logtest "knative.dev/pkg/logging/testing"
	pkgtest "knative.dev/pkg/reconciler/testing"

	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/kubernetes/fake"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"gotest.tools/assert"
	kubev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
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
		TypeMeta: v1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.internal.knative.dev/v1alpha1",
		},
		ObjectMeta: v1.ObjectMeta{
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
	// Create the Kubernetes services associated to the Knative services that
	// appear in the ingress above
	if err := createServicesWithNames(
		kubeClient,
		[]string{"hello-world-rev1", "hello-world-rev2"},
		"default",
	); err != nil {
		t.Error(err)
	}

	ingressTranslator := NewIngressTranslator(
		kubeClient, newMockedEndpointsLister(), "cluster.local", &pkgtest.FakeTracker{}, logtest.TestLogger(t))

	ingressTranslation, err := ingressTranslator.translateIngress(&ingress, false)
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

func TestIngressWithTLS(t *testing.T) {
	// TLS data for the test
	tlsSecretName := "tls-secret"
	tlsSecretNamespace := "default"
	tlsHosts := []string{"hello-world.example.com"}
	tlsCert := "some-cert"
	tlsKey := "tls-key"
	svcNamespace := "default"

	ingress := createIngressWithTLS(tlsHosts, tlsSecretName, tlsSecretNamespace, svcNamespace)

	kubeClient := fake.NewSimpleClientset()

	// Create the Kubernetes services associated to the Knative service that
	// appears in the ingress above
	if err := createServicesWithNames(kubeClient, []string{ingress.ObjectMeta.Name}, svcNamespace); err != nil {
		t.Error(err)
	}

	// Create secret with TLS data
	err := createSecret(kubeClient, tlsSecretNamespace, tlsSecretName, tlsCert, tlsKey)
	if err != nil {
		t.Error(err)
	}

	ingressTranslator := NewIngressTranslator(
		kubeClient, newMockedEndpointsLister(), "cluster.local", &pkgtest.FakeTracker{}, logtest.TestLogger(t))

	translatedIngress, err := ingressTranslator.translateIngress(ingress, false)
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

func newMockedEndpointsLister() corev1listers.EndpointsLister {
	return new(endpointsLister)
}

type endpointsLister struct{}

func (endpointsLister *endpointsLister) List(selector labels.Selector) ([]*kubev1.Endpoints, error) {
	return []*kubev1.Endpoints{{}}, nil
}

func (endpointsLister *endpointsLister) Endpoints(namespace string) corev1listers.EndpointsNamespaceLister {
	return new(endpoints)
}

type endpoints struct{}

func (endpoints *endpoints) List(selector labels.Selector) ([]*kubev1.Endpoints, error) {
	return []*kubev1.Endpoints{{}}, nil
}

func (endpoints *endpoints) Get(name string) (*kubev1.Endpoints, error) {
	return &kubev1.Endpoints{}, nil
}

func createIngressWithTLS(hosts []string, secretName string, secretNamespace string, svcNamespace string) *v1alpha1.Ingress {
	return &v1alpha1.Ingress{
		TypeMeta: v1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.internal.knative.dev/v1alpha1",
		},
		ObjectMeta: v1.ObjectMeta{
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
			Visibility: "",
		},
		Status: v1alpha1.IngressStatus{},
	}

}

func createSecret(kubeClient kubernetes.Interface, secretNamespace string, secretName string, cert string, key string) error {
	tlsSecret := kubev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name: secretName,
		},
		Data: map[string][]byte{
			certFieldInSecret: []byte(cert),
			keyFieldInSecret:  []byte(key),
		},
	}

	_, err := kubeClient.CoreV1().Secrets(secretNamespace).Create(&tlsSecret)
	return err
}

func createServicesWithNames(kubeclient kubernetes.Interface, names []string, namespace string) error {
	for _, serviceName := range names {
		service := kubev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name: serviceName,
			},
		}

		_, err := kubeclient.CoreV1().Services(namespace).Create(&service)

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
