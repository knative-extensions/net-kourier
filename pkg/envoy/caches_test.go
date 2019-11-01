package envoy

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"gotest.tools/assert"
	kubev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	// Check that there is one route in the result
	caches := CachesForClusterIngresses(
		[]v1alpha1.IngressAccessor{v1alpha1.IngressAccessor(&ingress)},
		newMockedKubeClient(),
		"cluster.local",
	)
	assert.Equal(t, 1, len(caches.routes))

	// Check that there are 2 weighted clusters for the route
	envoyRoute := caches.routes[0].(*route.Route)
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
		clustersExist([]string{"hello-world-rev1/", "hello-world-rev2/"}, caches.clusters),
	)
}

type mockedKubeClient struct{}

func (kubeClient *mockedKubeClient) EndpointsForRevision(namespace string, serviceName string) (*kubev1.EndpointsList, error) {
	list := kubev1.EndpointsList{
		Items: []kubev1.Endpoints{},
	}

	return &list, nil
}

func (kubeClient *mockedKubeClient) ServiceForRevision(namespace string, serviceName string) (*kubev1.Service, error) {
	service := kubev1.Service{
		Spec: kubev1.ServiceSpec{},
	}
	return &service, nil
}

func (kubeClient *mockedKubeClient) GetSecret(namespace string, secretName string) (*kubev1.Secret, error) {
	return nil, nil
}

func newMockedKubeClient() *mockedKubeClient {
	return new(mockedKubeClient)
}

func clustersExist(names []string, clustersCache []cache.Resource) bool {
	// Cast Resources to Clusters
	var clusters []*v2.Cluster
	for _, cacheCluster := range clustersCache {
		clusters = append(clusters, cacheCluster.(*v2.Cluster))
	}

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
