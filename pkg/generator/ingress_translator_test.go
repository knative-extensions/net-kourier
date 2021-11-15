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

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
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
					"(simplens/simplename).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(simplens/simplename).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
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
			secret,
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: vHosts,
				internalVirtualHosts:    vHosts,
			}
		}(),
	}, {
		name: "tls redirect",
		in: ing("testspace", "testname", func(ing *v1alpha1.Ingress) {
			ing.Spec.TLS = []v1alpha1.IngressTLS{{
				Hosts:           []string{"foo.example.com"},
				SecretNamespace: "secretns",
				SecretName:      "secretname",
			}}
			ing.Spec.HTTPOption = v1alpha1.HTTPOptionRedirected
		}),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
			secret,
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
						},
						0,
						map[string]string{"foo": "bar"},
						"rewritten.example.com"),
					},
				),
			}
			vHostsRedirect := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRedirectRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test"),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHostsRedirect,
				externalTLSVirtualHosts: vHosts,
				internalVirtualHosts:    vHostsRedirect,
			}
		}(),
	}, {
		// cluster local is not affected by HTTPOption.
		name: "tls redirect cluster local",
		in: ing("testspace", "testname", func(ing *v1alpha1.Ingress) {
			ing.Spec.TLS = []v1alpha1.IngressTLS{{
				Hosts:           []string{"foo.example.com"},
				SecretNamespace: "secretns",
				SecretName:      "secretname",
			}}
			ing.Spec.HTTPOption = v1alpha1.HTTPOptionRedirected
			ing.Spec.Rules[0].Visibility = v1alpha1.IngressVisibilityClusterLocal
		}),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
			secret,
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    []*route.VirtualHost{},
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
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
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 33, map[string]string{"baz": "gna"}),
							envoy.NewWeightedCluster("servicens2/servicename2", 33, nil),
							envoy.NewWeightedCluster("servicens3/servicename3", 34, nil),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
					envoy.NewCluster(
						"servicens2/servicename2",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
					envoy.NewCluster(
						"servicens3/servicename3",
						5*time.Second,
						[]*endpoint.LbEndpoint{envoy.NewLBEndpoint("example.com", 80)},
						false,
						v3.Cluster_LOGICAL_DNS,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
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
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
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
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						[]*endpoint.LbEndpoint{envoy.NewLBEndpoint("example.com", 80)},
						false,
						v3.Cluster_LOGICAL_DNS,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
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

// TestIngressTranslatorWithHTTPOptionDisabled runs same redirect test in TestIngressTranslator with KOURIER_HTTPOPTION_DISABLED env value.
func TestIngressTranslatorWithHTTPOptionDisabled(t *testing.T) {
	tests := []struct {
		name  string
		in    *v1alpha1.Ingress
		state []runtime.Object
		want  *translatedIngress
	}{{
		name: "tls redirect",
		in: ing("testspace", "testname", func(ing *v1alpha1.Ingress) {
			ing.Spec.TLS = []v1alpha1.IngressTLS{{
				Hosts:           []string{"foo.example.com"},
				SecretNamespace: "secretns",
				SecretName:      "secretname",
			}}
			ing.Spec.HTTPOption = v1alpha1.HTTPOptionRedirected
		}),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
			secret,
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: vHosts,
				internalVirtualHosts:    vHosts,
			}
		}(),
	}, {
		// cluster local is not affected by HTTPOption.
		name: "tls redirect cluster local",
		in: ing("testspace", "testname", func(ing *v1alpha1.Ingress) {
			ing.Spec.TLS = []v1alpha1.IngressTLS{{
				Hosts:           []string{"foo.example.com"},
				SecretNamespace: "secretns",
				SecretName:      "secretname",
			}}
			ing.Spec.HTTPOption = v1alpha1.HTTPOptionRedirected
			ing.Spec.Rules[0].Visibility = v1alpha1.IngressVisibilityClusterLocal
		}),
		state: []runtime.Object{
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
			secret,
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHost(
					"(testspace/testname).Rules[0]",
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRoute(
						"(testspace/testname).Rules[0].Paths[/test]",
						[]*route.HeaderMatcher{{
							Name: "testheader",
							HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
								ExactMatch: "foo",
							},
						}},
						"/test",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("servicens/servicename", 100, map[string]string{"baz": "gna"}),
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
				clusters: []*v3.Cluster{
					envoy.NewCluster(
						"servicens/servicename",
						5*time.Second,
						lbEndpoints,
						false,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    []*route.VirtualHost{},
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("KOURIER_HTTPOPTION_DISABLED", "true")
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

var (
	cert       = []byte("cert")
	privateKey = []byte("key")
	secret     = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "secretns",
			Name:      "secretname",
		},
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": privateKey,
		},
	}
)
