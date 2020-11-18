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
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	pkgtest "knative.dev/pkg/reconciler/testing"
)

func TestIngressTranslator(t *testing.T) {
	tests := []struct {
		name  string
		in    *v1alpha1.Ingress
		state []runtime.Object
		want  *translatedIngress
	}{{
		name: "simple",
		in:   ing("simplens", "simplename"),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"simplename",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"simplename_simplens_/test",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicename/test", 100, map[string]string{"baz": "gna"}),
						},
						0,
						map[string]string{"foo": "bar"},
						"rewritten.example.com"),
					},
				),
			}

			return &translatedIngress{
				name: types.NamespacedName{
					Namespace: "simplens",
					Name:      "simplename",
				},
				sniMatches: []*envoy.SNIMatch{},
				clusters: []*v2.Cluster{
					envoy.NewCluster(
						"servicename/test",
						5*time.Second,
						lbEndpoints,
						false,
						v2.Cluster_STATIC,
					),
				},
				externalVirtualHosts: vHosts,
				internalVirtualHosts: vHosts,
			}
		}(),
	}, {
		name: "tls",
		in: ing("testspace", "testname", func(ing *v1alpha1.Ingress) {
			ing.Spec.TLS = []v1alpha1.IngressTLS{{
				Hosts:           []string{"foo.example.com"},
				SecretNamespace: "secretns",
				SecretName:      "secretname",
			}}
		}),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
			secret("secretns", "secretname"),
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"testname",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"testname_testspace_/test",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicename/test", 100, map[string]string{"baz": "gna"}),
						},
						0,
						map[string]string{"foo": "bar"},
						"rewritten.example.com"),
					},
				),
			}

			return &translatedIngress{
				name: types.NamespacedName{
					Namespace: "testspace",
					Name:      "testname",
				},
				sniMatches: []*envoy.SNIMatch{{
					Hosts: []string{"foo.example.com"},
					CertSource: types.NamespacedName{
						Namespace: "secretns",
						Name:      "secretname",
					},
					CertificateChain: cert,
					PrivateKey:       privateKey,
				}},
				clusters: []*v2.Cluster{
					envoy.NewCluster(
						"servicename/test",
						5*time.Second,
						lbEndpoints,
						false,
						v2.Cluster_STATIC,
					),
				},
				externalVirtualHosts: vHosts,
				internalVirtualHosts: vHosts,
			}
		}(),
	}, {
		name: "split",
		in: ing("testspace", "testname", func(ing *v1alpha1.Ingress) {
			path := &ing.Spec.Rules[0].HTTP.Paths[0]
			path.Splits[0].Percent = 33
			path.Splits = append(path.Splits, v1alpha1.IngressBackendSplit{
				Percent: 33,
				IngressBackend: v1alpha1.IngressBackend{
					ServiceNamespace: "servicens2",
					ServiceName:      "servicename2",
					ServicePort:      intstr.FromString("http"),
				},
			}, v1alpha1.IngressBackendSplit{
				Percent: 34,
				IngressBackend: v1alpha1.IngressBackend{
					ServiceNamespace: "servicens3",
					ServiceName:      "servicename3",
					ServicePort:      intstr.FromString("http"),
				},
			})
		}),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
			svc("servicens2", "servicename2"),
			eps("servicens2", "servicename2"),
			svc("servicens3", "servicename3", func(svc *corev1.Service) {
				svc.Spec.Type = corev1.ServiceTypeExternalName
				svc.Spec.ExternalName = "example.com"
			}),
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"testname",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"testname_testspace_/test",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicename/test", 33, map[string]string{"baz": "gna"}),
							envoy.NewWeightedCluster("servicename2/test", 33, nil),
							envoy.NewWeightedCluster("servicename3/test", 34, nil),
						},
						0,
						map[string]string{"foo": "bar"},
						"rewritten.example.com"),
					},
				),
			}

			return &translatedIngress{
				name: types.NamespacedName{
					Namespace: "testspace",
					Name:      "testname",
				},
				sniMatches: []*envoy.SNIMatch{},
				clusters: []*v2.Cluster{
					envoy.NewCluster(
						"servicename/test",
						5*time.Second,
						lbEndpoints,
						false,
						v2.Cluster_STATIC,
					),
					envoy.NewCluster(
						"servicename2/test",
						5*time.Second,
						lbEndpoints,
						false,
						v2.Cluster_STATIC,
					),
					envoy.NewCluster(
						"servicename3/test",
						5*time.Second,
						[]*endpoint.LbEndpoint{envoy.NewLBEndpoint("example.com", 80)},
						false,
						v2.Cluster_LOGICAL_DNS,
					),
				},
				externalVirtualHosts: vHosts,
				internalVirtualHosts: vHosts,
			}
		}(),
	}, {
		name: "path defaulting",
		in: ing("testspace", "testname", func(ing *v1alpha1.Ingress) {
			ing.Spec.Rules[0].HTTP.Paths[0].Path = ""
		}),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"testname",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"testname_testspace_",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicename/", 100, map[string]string{"baz": "gna"}),
						},
						0,
						map[string]string{"foo": "bar"},
						"rewritten.example.com"),
					},
				),
			}

			return &translatedIngress{
				name: types.NamespacedName{
					Namespace: "testspace",
					Name:      "testname",
				},
				sniMatches: []*envoy.SNIMatch{},
				clusters: []*v2.Cluster{
					envoy.NewCluster(
						"servicename/",
						5*time.Second,
						lbEndpoints,
						false,
						v2.Cluster_STATIC,
					),
				},
				externalVirtualHosts: vHosts,
				internalVirtualHosts: vHosts,
			}
		}(),
	}, {
		name: "external service",
		in:   ing("testspace", "testname"),
		state: []runtime.Object{
			svc("servicens", "servicename", func(svc *corev1.Service) {
				svc.Spec.Type = corev1.ServiceTypeExternalName
				svc.Spec.ExternalName = "example.com"
			}),
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"testname",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"testname_testspace_/test",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicename/test", 100, map[string]string{"baz": "gna"}),
						},
						0,
						map[string]string{"foo": "bar"},
						"rewritten.example.com"),
					},
				),
			}

			return &translatedIngress{
				name: types.NamespacedName{
					Namespace: "testspace",
					Name:      "testname",
				},
				sniMatches: []*envoy.SNIMatch{},
				clusters: []*v2.Cluster{
					envoy.NewCluster(
						"servicename/test",
						5*time.Second,
						[]*endpoint.LbEndpoint{envoy.NewLBEndpoint("example.com", 80)},
						false,
						v2.Cluster_LOGICAL_DNS,
					),
				},
				externalVirtualHosts: vHosts,
				internalVirtualHosts: vHosts,
			}
		}(),
	}, {
		name: "missing service",
		in:   ing("testspace", "testname"),
	}, {
		name:  "missing endpoints",
		in:    ing("testspace", "testname"),
		state: []runtime.Object{svc("servicens", "servicename")},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := pkgtest.SetupFakeContext(t)
			kubeclient := fake.NewSimpleClientset(test.state...)

			translator := NewIngressTranslator(
				func(ns, name string) (*corev1.Secret, error) {
					return kubeclient.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
				},
				func(ns, name string) (*corev1.Endpoints, error) {
					return kubeclient.CoreV1().Endpoints(ns).Get(ctx, name, metav1.GetOptions{})
				},
				func(ns, name string) (*corev1.Service, error) {
					return kubeclient.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
				},
				&pkgtest.FakeTracker{},
			)

			got, err := translator.translateIngress(ctx, test.in, false)
			assert.NilError(t, err)
			assert.DeepEqual(t, got, test.want,
				cmp.AllowUnexported(translatedIngress{}),
				protocmp.Transform(),
			)
		})
	}
}

func ing(ns, name string, opts ...func(*v1alpha1.Ingress)) *v1alpha1.Ingress {
	ingress := &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.IngressRule{{
				Hosts:      []string{"foo.example.com"},
				Visibility: v1alpha1.IngressVisibilityExternalIP,
				HTTP: &v1alpha1.HTTPIngressRuleValue{
					Paths: []v1alpha1.HTTPIngressPath{{
						RewriteHost: "rewritten.example.com",
						Headers: map[string]v1alpha1.HeaderMatch{
							"testheader": {Exact: "foo"},
						},
						Path: "/test",
						AppendHeaders: map[string]string{
							"foo": "bar",
						},
						Splits: []v1alpha1.IngressBackendSplit{{
							Percent: 100,
							AppendHeaders: map[string]string{
								"baz": "gna",
							},
							IngressBackend: v1alpha1.IngressBackend{
								ServiceNamespace: "servicens",
								ServiceName:      "servicename",
								ServicePort:      intstr.FromString("http"),
							},
						}},
					}},
				},
			}},
		},
	}

	for _, opt := range opts {
		opt(ingress)
	}

	return ingress
}

func svc(ns, name string, opts ...func(*corev1.Service)) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "1.1.1.1",
			Ports: []corev1.ServicePort{{
				Name:       "foo",
				Port:       1337,
				TargetPort: intstr.FromInt(1338),
			}, {
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

func eps(ns, name string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{
				IP: "2.2.2.2",
			}, {
				IP: "3.3.3.3",
			}},
		}, {
			Addresses: []corev1.EndpointAddress{{
				IP: "4.4.4.4",
			}, {
				IP: "5.5.5.5",
			}},
		}},
	}
}

var lbEndpoints = []*endpoint.LbEndpoint{
	envoy.NewLBEndpoint("2.2.2.2", 8080),
	envoy.NewLBEndpoint("3.3.3.3", 8080),
	envoy.NewLBEndpoint("4.4.4.4", 8080),
	envoy.NewLBEndpoint("5.5.5.5", 8080),
}

func secret(ns, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": privateKey,
		},
	}
}

var (
	cert       = []byte("cert")
	privateKey = []byte("key")
)
