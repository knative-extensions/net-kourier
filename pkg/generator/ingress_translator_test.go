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
	"testing"
	"time"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	auth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"knative.dev/control-protocol/pkg/certificates"
	pkgconfig "knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	netconfig "knative.dev/networking/pkg/config"
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
			ns("simplens"),
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
						nil,
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
			ns("testspace"),
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
						nil,
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
			ns("testspace"),
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
						nil,
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
			ns("testspace"),
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
						nil,
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
			ns("testspace"),
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
						nil,
						v3.Cluster_STATIC,
					),
					envoy.NewCluster(
						"servicens2/servicename2",
						5*time.Second,
						lbEndpoints,
						false,
						nil,
						v3.Cluster_STATIC,
					),
					envoy.NewCluster(
						"servicens3/servicename3",
						5*time.Second,
						[]*endpoint.LbEndpoint{envoy.NewLBEndpoint("example.com", 80)},
						false,
						nil,
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
			ns("testspace"),
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
						nil,
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
			ns("testspace"),
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
						nil,
						v3.Cluster_LOGICAL_DNS,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}, {
		name: "external service without service port",
		in:   ing("testspace", "testname"),
		state: []runtime.Object{
			ns("testspace"),
			svc("servicens", "servicename", func(svc *corev1.Service) {
				svc.Spec.Type = corev1.ServiceTypeExternalName
				svc.Spec.ExternalName = "example.com"
				svc.Spec.Ports = nil
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
						nil,
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
			cfg := defaultConfig.DeepCopy()
			ctx := (&testConfigStore{config: cfg}).ToContext(context.Background())

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
				func(name string) (*corev1.Namespace, error) {
					return kubeclient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
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

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

var (
	defaultConfig = &config.Config{
		Network: &netconfig.Config{
			AutoTLS: false,
		},
		Kourier: pkgconfig.DefaultConfig(),
	}
	upstreamTLSConfig = &config.Config{
		Network: &netconfig.Config{
			AutoTLS: false,
		},
		Kourier: pkgconfig.DefaultConfig(),
	}
)

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
			ns("testspace"),
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
						nil,
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
			ns("testspace"),
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
						nil,
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
			cfg := defaultConfig.DeepCopy()
			ctx := (&testConfigStore{config: cfg}).ToContext(context.Background())
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
				func(name string) (*corev1.Namespace, error) {
					return kubeclient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
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

func TestIngressTranslatorWithUpstreamTLS(t *testing.T) {
	tests := []struct {
		name  string
		in    *v1alpha1.Ingress
		state []runtime.Object
		want  *translatedIngress
	}{{
		name: "simple",
		in: ing("simplens", "simplename", func(ing *v1alpha1.Ingress) {
			ing.Spec.Rules[0].HTTP.Paths[0].RewriteHost = ""
			ing.Spec.Rules[0].HTTP.Paths[0].Splits[0].IngressBackend.ServicePort = intstr.FromInt(443)
		}),
		state: []runtime.Object{
			ns("simplens"),
			svc("servicens", "servicename"),
			eps("servicens", "servicename"),
			caSecret,
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
						""),
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
						lbHTTPSEndpoints,
						false,
						&envoycorev3.TransportSocket{
							Name:       wellknown.TransportSocketTls,
							ConfigType: typedConfig(false /* http2 */),
						},
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}, {
		name: "http2",
		in: ing("simplens", "simplename", func(ing *v1alpha1.Ingress) {
			ing.Spec.Rules[0].HTTP.Paths[0].RewriteHost = ""
			ing.Spec.Rules[0].HTTP.Paths[0].Splits[0].IngressBackend.ServicePort = intstr.FromInt(443)
		}),
		state: []runtime.Object{
			ns("simplens"),
			svc("servicens", "servicename", func(service *corev1.Service) {
				service.Spec.Ports = []corev1.ServicePort{{
					Name:       "http2",
					TargetPort: intstr.FromInt(8080),
				}, {
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				}}
			}),
			eps("servicens", "servicename"),
			caSecret,
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
						""),
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
						lbHTTPSEndpoints,
						true, /* http2 */
						&envoycorev3.TransportSocket{
							Name:       wellknown.TransportSocketTls,
							ConfigType: typedConfig(true /* http2 */),
						},
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}, {
		name: "http and https",
		in: ing("simplens", "simplename", func(ing *v1alpha1.Ingress) {
			ing.Spec.Rules[0].HTTP.Paths[0].RewriteHost = ""
			ing.Spec.Rules[0].HTTP.Paths[0].Splits[0].IngressBackend.ServicePort = intstr.FromInt(443)
		}),
		state: []runtime.Object{
			ns("simplens"),
			svc("servicens", "servicename", func(service *corev1.Service) {
				service.Spec.Ports = []corev1.ServicePort{{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				}, {
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				}}
			}),
			eps("servicens", "servicename"),
			caSecret,
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
						""),
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
						lbHTTPSEndpoints,
						false, /* http2 */
						&envoycorev3.TransportSocket{
							Name:       wellknown.TransportSocketTls,
							ConfigType: typedConfig(false /* http2 */),
						},
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}, {
		name: "http2 and https",
		in: ing("simplens", "simplename", func(ing *v1alpha1.Ingress) {
			ing.Spec.Rules[0].HTTP.Paths[0].RewriteHost = ""
			ing.Spec.Rules[0].HTTP.Paths[0].Splits[0].IngressBackend.ServicePort = intstr.FromInt(443)
		}),
		state: []runtime.Object{
			ns("simplens"),
			svc("servicens", "servicename", func(service *corev1.Service) {
				service.Spec.Ports = []corev1.ServicePort{{
					Name:       "http2",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				}, {
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				}}
			}),
			eps("servicens", "servicename"),
			caSecret,
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
						""),
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
						lbHTTPSEndpoints,
						true, /* http2 */
						&envoycorev3.TransportSocket{
							Name:       wellknown.TransportSocketTls,
							ConfigType: typedConfig(true /* http2 */),
						},
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := upstreamTLSConfig.DeepCopy()
			ctx := (&testConfigStore{config: cfg}).ToContext(context.Background())

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
				func(name string) (*corev1.Namespace, error) {
					return kubeclient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
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

func TestIngressTranslatorHTTP01Challenge(t *testing.T) {
	test := struct {
		name  string
		in    *v1alpha1.Ingress
		state []runtime.Object
		want  *translatedIngress
	}{
		name: "http01-challenge",
		in:   ingHTTP01Challenge("simplens", "simplename"),
		state: []runtime.Object{
			ns("simplens"),
			svc("simplens", "cm-acme-http-solver", func(service *corev1.Service) {
				service.Spec.Ports = []corev1.ServicePort{{
					Name:       "http01-challenge",
					TargetPort: intstr.FromInt(8089),
				}}
			}),
			eps("simplens", "cm-acme-http-solver", func(endpoint *corev1.Endpoints) {
				endpoint.Subsets = []corev1.EndpointSubset{{
					Addresses: []corev1.EndpointAddress{{
						IP: "2.2.2.2",
					}},
				}}
			}),
		},
		want: func() *translatedIngress {
			vHosts := []*route.VirtualHost{
				envoy.NewVirtualHostWithExtAuthz(
					"(simplens/simplename).Rules[0]",
					map[string]string{"client": "kourier", "visibility": "ExternalIP"},
					[]string{"foo.example.com", "foo.example.com:*"},
					[]*route.Route{envoy.NewRouteExtAuthzDisabled(
						"(simplens/simplename).Rules[0].Paths[/.well-known/acme-challenge/-VwB1vAXWaN6mVl3-6JVFTEvf7acguaFDUxsP9UzRkE]",
						nil,
						"/.well-known/acme-challenge/-VwB1vAXWaN6mVl3-6JVFTEvf7acguaFDUxsP9UzRkE",
						[]*route.WeightedCluster_ClusterWeight{
							envoy.NewWeightedCluster("simplens/cm-acme-http-solver", 100, nil),
						},
						0,
						nil,
						""),
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
						"simplens/cm-acme-http-solver",
						5*time.Second,
						lbEndpointHTTP01Challenge,
						false,
						nil,
						v3.Cluster_STATIC,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}

	t.Run(test.name, func(t *testing.T) {
		ctx, _ := pkgtest.SetupFakeContext(t)
		cfg := defaultConfig.DeepCopy()
		ctx = (&testConfigStore{config: cfg}).ToContext(ctx)

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
			func(name string) (*corev1.Namespace, error) {
				return kubeclient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
			},
			&pkgtest.FakeTracker{},
		)

		got, err := translator.translateIngress(ctx, test.in, true)

		assert.NilError(t, err)
		assert.DeepEqual(t, got, test.want,
			cmp.AllowUnexported(translatedIngress{}),
			protocmp.Transform(),
		)
	})
}

func TestIngressTranslatorDomainMappingDisableHTTP2(t *testing.T) {
	test := struct {
		name  string
		in    *v1alpha1.Ingress
		state []runtime.Object
		want  *translatedIngress
	}{
		name: "disable http2",
		in: ing("simplens", "simplename", func(ing *v1alpha1.Ingress) {
			ing.Annotations = map[string]string{"kourier.knative.dev/disable-http2": "true"}
			ing.Spec.Rules[0].HTTP.Paths[0].RewriteHost = "bar.default.svc.cluster.local"
			ing.Spec.Rules[0].HTTP.Paths[0].Splits[0].ServicePort = intstr.FromInt(80)
		}),
		state: []runtime.Object{
			ns("simplens"),
			svc("servicens", "servicename", func(service *corev1.Service) {
				service.Spec.Type = corev1.ServiceTypeExternalName
				service.Spec.ExternalName = "kourier-internal.kourier-system.svc.cluster.local"
				service.Spec.Ports = []corev1.ServicePort{{
					Name:       "http2",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
				}}
			}),
			eps("servicens", "servicename"),
			caSecret,
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
						"bar.default.svc.cluster.local"),
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
						[]*endpoint.LbEndpoint{
							envoy.NewLBEndpoint("kourier-internal.kourier-system.svc.cluster.local", 80),
						},
						false, /* http2 */
						nil,
						v3.Cluster_LOGICAL_DNS,
					),
				},
				externalVirtualHosts:    vHosts,
				externalTLSVirtualHosts: []*route.VirtualHost{},
				internalVirtualHosts:    vHosts,
			}
		}(),
	}

	t.Run(test.name, func(t *testing.T) {
		cfg := upstreamTLSConfig.DeepCopy()
		ctx := (&testConfigStore{config: cfg}).ToContext(context.Background())

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
			func(name string) (*corev1.Namespace, error) {
				return kubeclient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
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
			}, {
				Name:       "https",
				Port:       443,
				TargetPort: intstr.FromInt(8443),
			}},
		},
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

func eps(ns, name string, opts ...func(endpoint *corev1.Endpoints)) *corev1.Endpoints {
	serviceEndpoint := &corev1.Endpoints{
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

	for _, opt := range opts {
		opt(serviceEndpoint)
	}

	return serviceEndpoint
}

func ns(name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	return ns
}

func ingHTTP01Challenge(ns, name string, opts ...func(*v1alpha1.Ingress)) *v1alpha1.Ingress {
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
						Path: "/.well-known/acme-challenge/-VwB1vAXWaN6mVl3-6JVFTEvf7acguaFDUxsP9UzRkE",
						Splits: []v1alpha1.IngressBackendSplit{{
							Percent: 100,
							IngressBackend: v1alpha1.IngressBackend{
								ServiceNamespace: ns,
								ServiceName:      "cm-acme-http-solver",
								ServicePort:      intstr.FromString("http01-challenge"),
							}},
						},
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

var lbEndpoints = []*endpoint.LbEndpoint{
	envoy.NewLBEndpoint("2.2.2.2", 8080),
	envoy.NewLBEndpoint("3.3.3.3", 8080),
	envoy.NewLBEndpoint("4.4.4.4", 8080),
	envoy.NewLBEndpoint("5.5.5.5", 8080),
}

var lbHTTPSEndpoints = []*endpoint.LbEndpoint{
	envoy.NewLBEndpoint("2.2.2.2", 8443),
	envoy.NewLBEndpoint("3.3.3.3", 8443),
	envoy.NewLBEndpoint("4.4.4.4", 8443),
	envoy.NewLBEndpoint("5.5.5.5", 8443),
}

var lbEndpointHTTP01Challenge = []*endpoint.LbEndpoint{
	envoy.NewLBEndpoint("2.2.2.2", 8089),
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
	caSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "knative-testing",
			Name:      netconfig.ServingInternalCertName,
		},
		Data: map[string][]byte{
			certificates.SecretCaCertKey: cert,
		},
	}
)

func typedConfig(http2 bool) *envoycorev3.TransportSocket_TypedConfig {
	alpn := []string{""}
	if http2 {
		alpn = []string{"h2"}
	}
	tlsAny, _ := anypb.New(&auth.UpstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			AlpnProtocols: alpn,
			TlsParams: &auth.TlsParameters{
				TlsMinimumProtocolVersion: auth.TlsParameters_TLSv1_2,
			},
			ValidationContextType: &auth.CommonTlsContext_ValidationContext{
				ValidationContext: &auth.CertificateValidationContext{
					TrustedCa: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineBytes{
							InlineBytes: cert,
						},
					},
					MatchSubjectAltNames: []*envoymatcherv3.StringMatcher{{
						MatchPattern: &envoymatcherv3.StringMatcher_Exact{
							Exact: certificates.FakeDnsName,
						}},
					},
				},
			},
		},
	})
	return &envoycorev3.TransportSocket_TypedConfig{
		TypedConfig: tlsAny,
	}
}
