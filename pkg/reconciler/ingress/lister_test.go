/*
Copyright 2025 The Knative Authors

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

package ingress

import (
	"context"
	"errors"
	"log"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"knative.dev/net-kourier/pkg/config"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/kmeta"

	"go.uber.org/zap/zaptest"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

const (
	gatewayNamespace = "default"
)

func TestListProveTargets(t *testing.T) {
	tests := []struct {
		name            string
		endpointsLister corev1listers.EndpointsLister
		ingress         *v1alpha1.Ingress
		errMessage      string
		results         []status.ProbeTarget
	}{
		{
			name: "endpoints error",
			endpointsLister: &fakeEndpointsLister{
				fails: true,
			},
			ingress:    &v1alpha1.Ingress{},
			errMessage: "failed to get internal service:",
		},
		{
			name: "not found intertnal service name",
			endpointsLister: &fakeEndpointsLister{
				endpointses: []*v1.Endpoints{{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: gatewayNamespace,
						Name:      "not-internal-service-name",
					},
				}},
			},
			errMessage: "failed to get internal service:",
		},
		{
			name: "subset with no addresses",
			endpointsLister: &fakeEndpointsLister{
				endpointses: []*v1.Endpoints{{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      config.InternalServiceName,
					},
					Subsets: []v1.EndpointSubset{{
						Ports: []v1.EndpointPort{{
							Name: "bogus",
							Port: 8080,
						}},
					}},
				}},
			},
			errMessage: "no gateway pods available",
		},
		{
			name: "externalIP and externalTLS",
			endpointsLister: &fakeEndpointsLister{
				endpointses: []*v1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName,
						},
						Subsets: []v1.EndpointSubset{{
							Ports: []v1.EndpointPort{},
							Addresses: []v1.EndpointAddress{{
								IP: "1.1.1.1",
							}},
						}},
					},
				},
			},
			ingress: ing("ing", gatewayNamespace,
				withRule([]string{"foo.bar.com"}, v1alpha1.IngressVisibilityExternalIP),
				withTLS([]string{"foo.bar.com"}),
			),
			results: []status.ProbeTarget{{
				PodIPs:  sets.New("1.1.1.1"),
				PodPort: "9443",
				URLs:    []*url.URL{{Scheme: "https", Host: "foo.bar.com", Path: "/"}},
			}},
		},
		{
			name: "externalIP and not externalTLS",
			endpointsLister: &fakeEndpointsLister{
				endpointses: []*v1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName,
						},
						Subsets: []v1.EndpointSubset{{
							Ports: []v1.EndpointPort{},
							Addresses: []v1.EndpointAddress{{
								IP: "1.1.1.1",
							}},
						}},
					},
				},
			},
			ingress: ing("ing", gatewayNamespace,
				withRule([]string{"foo.bar.com"}, v1alpha1.IngressVisibilityExternalIP),
			),
			results: []status.ProbeTarget{{
				PodIPs:  sets.New("1.1.1.1"),
				PodPort: "8090",
				URLs:    []*url.URL{{Scheme: "http", Host: "foo.bar.com", Path: "/"}},
			}},
		},
		{
			name: "clousterLocal and localTLS",
			endpointsLister: &fakeEndpointsLister{
				endpointses: []*v1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName,
						},
						Subsets: []v1.EndpointSubset{{
							Ports: []v1.EndpointPort{},
							Addresses: []v1.EndpointAddress{{
								IP: "1.1.1.1",
							}},
						}},
					},
				},
			},
			ingress: ing("ing", gatewayNamespace,
				withRule([]string{"foo.bar.com"}, v1alpha1.IngressVisibilityClusterLocal),
				withTLS([]string{"foo.bar.com"}),
			),
			results: []status.ProbeTarget{{
				PodIPs:  sets.New("1.1.1.1"),
				PodPort: "8444",
				URLs:    []*url.URL{{Scheme: "https", Host: "foo.bar.com", Path: "/"}},
			}},
		},
		{
			name: "clousterLocal and not localTLS",
			endpointsLister: &fakeEndpointsLister{
				endpointses: []*v1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName,
						},
						Subsets: []v1.EndpointSubset{{
							Ports: []v1.EndpointPort{},
							Addresses: []v1.EndpointAddress{{
								IP: "1.1.1.1",
							}},
						}},
					},
				},
			},
			ingress: ing("ing", gatewayNamespace,
				withRule([]string{"foo.bar.com"}, v1alpha1.IngressVisibilityClusterLocal),
			),
			results: []status.ProbeTarget{{
				PodIPs:  sets.New("1.1.1.1"),
				PodPort: "8081",
				URLs:    []*url.URL{{Scheme: "http", Host: "foo.bar.com", Path: "/"}},
			}},
		},
	}

	for _, test := range tests {
		t.Setenv(config.GatewayNamespaceEnv, gatewayNamespace)
		t.Run(test.name, func(t *testing.T) {
			lister := NewProbeTargetLister(
				zaptest.NewLogger(t).Sugar(),
				test.endpointsLister,
			)

			results, err := lister.ListProbeTargets(context.Background(), test.ingress)
			if err == nil {
				if test.errMessage != "" {
					t.Fatalf("expected error message %q, saw no error", test.errMessage)
				}
			} else if !strings.Contains(err.Error(), test.errMessage) {
				t.Errorf("expected error message %q but saw %v", test.errMessage, err)
			}

			if len(test.results)+len(results) > 0 { // consider nil map == empty map
				// Sort by port number
				sort.Slice(results, func(i, j int) bool {
					return results[i].Port < results[j].Port
				})
				if diff := cmp.Diff(test.results, results); diff != "" {
					t.Error("Unexpected probe targets (-want +got):", diff)
				}
			}
		})
	}

}

type fakeEndpointsLister struct {
	endpointses []*v1.Endpoints
	fails       bool
}

func (l *fakeEndpointsLister) List(_ labels.Selector) ([]*v1.Endpoints, error) {
	log.Panic("not implemented")
	return nil, nil
}

func (l *fakeEndpointsLister) Endpoints(_ string) corev1listers.EndpointsNamespaceLister {
	return l
}

func (l *fakeEndpointsLister) Get(name string) (*v1.Endpoints, error) {
	if l.fails {
		return nil, errors.New("failed to get Endpoints")
	}
	for _, ep := range l.endpointses {
		if ep.Name == name {
			return ep, nil
		}
	}
	return nil, errors.New("not found")
}

type ingressOption func(*v1alpha1.Ingress)

func ing(name, ns string, opts ...ingressOption) *v1alpha1.Ingress {
	i := &v1alpha1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1alpha1.IngressSpec{
			HTTPOption: v1alpha1.HTTPOptionEnabled,
		},
	}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

func withRule(hosts []string, visibility v1alpha1.IngressVisibility) ingressOption {
	return func(i *v1alpha1.Ingress) {
		i.Spec.Rules = append(i.Spec.Rules, v1alpha1.IngressRule{
			Hosts:      hosts,
			Visibility: visibility,
		})
	}
}

func withTLS(hosts []string) ingressOption {
	return func(i *v1alpha1.Ingress) {
		i.Spec.TLS = append(i.Spec.TLS, v1alpha1.IngressTLS{
			Hosts: hosts,
		})
	}
}

func withAnnotation(ann map[string]string) ingressOption {
	return func(i *v1alpha1.Ingress) {
		i.Annotations = kmeta.UnionMaps(i.Annotations, ann)
	}
}

func withKourier(i *v1alpha1.Ingress) {
	withAnnotation(map[string]string{
		networking.IngressClassAnnotationKey: config.KourierIngressClassName,
	})(i)
}

func withBasicSpec(i *v1alpha1.Ingress) {
	i.Spec = v1alpha1.IngressSpec{
		HTTPOption: v1alpha1.HTTPOptionEnabled,
		Rules: []v1alpha1.IngressRule{{
			Hosts:      []string{"example.com"},
			Visibility: v1alpha1.IngressVisibilityExternalIP,
			HTTP: &v1alpha1.HTTPIngressRuleValue{
				Paths: []v1alpha1.HTTPIngressPath{{
					Splits: []v1alpha1.IngressBackendSplit{{
						IngressBackend: v1alpha1.IngressBackend{
							ServiceName:      "goo",
							ServiceNamespace: i.Namespace,
							ServicePort:      intstr.FromInt(123),
						},
						Percent: 100,
					}},
				}},
			},
		}},
	}
}
