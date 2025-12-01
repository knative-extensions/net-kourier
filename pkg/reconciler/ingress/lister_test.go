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
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/kmeta"

	"go.uber.org/zap/zaptest"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	discoveryv1listers "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/utils/ptr"
)

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

func TestListProbeTargets(t *testing.T) {
	tests := []struct {
		name                 string
		endpointSlicesLister discoveryv1listers.EndpointSliceLister
		ingress              *v1alpha1.Ingress
		errMessage           string
		results              []status.ProbeTarget
	}{
		{
			name: "single slice with ready endpoints",
			endpointSlicesLister: &fakeEndpointSlicesLister{
				slices: []*discoveryv1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName + "-abc",
							Labels: map[string]string{
								discoveryv1.LabelServiceName: config.InternalServiceName,
							},
						},
						AddressType: discoveryv1.AddressTypeIPv4,
						Endpoints: []discoveryv1.Endpoint{
							{
								Addresses: []string{"127.0.0.1"},
								Conditions: discoveryv1.EndpointConditions{
									Ready: ptr.To(true),
								},
							},
						},
					},
				},
			},
			ingress: ingressWithVisibility(v1alpha1.IngressVisibilityExternalIP, "foo.bar.com", true),
			results: []status.ProbeTarget{{
				PodIPs:  sets.New("127.0.0.1"),
				PodPort: "9443",
				URLs:    []*url.URL{{Scheme: "https", Host: "foo.bar.com", Path: "/"}},
			}},
		},
		{
			name: "multiple slices aggregation",
			endpointSlicesLister: &fakeEndpointSlicesLister{
				slices: []*discoveryv1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName + "-abc",
							Labels: map[string]string{
								discoveryv1.LabelServiceName: config.InternalServiceName,
							},
						},
						AddressType: discoveryv1.AddressTypeIPv4,
						Endpoints: []discoveryv1.Endpoint{
							{
								Addresses: []string{"127.0.0.1"},
								Conditions: discoveryv1.EndpointConditions{
									Ready: ptr.To(true),
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName + "-xyz",
							Labels: map[string]string{
								discoveryv1.LabelServiceName: config.InternalServiceName,
							},
						},
						AddressType: discoveryv1.AddressTypeIPv4,
						Endpoints: []discoveryv1.Endpoint{
							{
								Addresses: []string{"127.0.0.2"},
								Conditions: discoveryv1.EndpointConditions{
									Ready: ptr.To(true),
								},
							},
						},
					},
				},
			},
			ingress: ingressWithVisibility(v1alpha1.IngressVisibilityExternalIP, "foo.bar.com", false),
			results: []status.ProbeTarget{{
				PodIPs:  sets.New("127.0.0.1", "127.0.0.2"),
				PodPort: "8090",
				URLs:    []*url.URL{{Scheme: "http", Host: "foo.bar.com", Path: "/"}},
			}},
		},
		{
			name: "no ready endpoints",
			endpointSlicesLister: &fakeEndpointSlicesLister{
				slices: []*discoveryv1.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      config.InternalServiceName + "-abc",
							Labels: map[string]string{
								discoveryv1.LabelServiceName: config.InternalServiceName,
							},
						},
						AddressType: discoveryv1.AddressTypeIPv4,
						Endpoints:   []discoveryv1.Endpoint{},
					},
				},
			},
			ingress:    &v1alpha1.Ingress{},
			errMessage: "no gateway pods available",
		},
	}

	for _, test := range tests {
		t.Setenv(config.GatewayNamespaceEnv, "default")
		t.Run(test.name, func(t *testing.T) {
			lister := NewProbeTargetLister(
				zaptest.NewLogger(t).Sugar(),
				test.endpointSlicesLister,
			)

			results, err := lister.ListProbeTargets(context.Background(), test.ingress)
			if err == nil {
				if test.errMessage != "" {
					t.Fatal("expected error but got none")
				}
			} else {
				if test.errMessage == "" {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(err.Error(), test.errMessage) {
					t.Fatalf("expected error containing %q, got %q", test.errMessage, err.Error())
				}
			}

			if diff := cmp.Diff(test.results, results); diff != "" {
				t.Error("Unexpected results (-want +got):", diff)
			}
		})
	}
}

type fakeEndpointSlicesLister struct {
	slices []*discoveryv1.EndpointSlice
	fails  bool
}

func (l *fakeEndpointSlicesLister) List(_ labels.Selector) ([]*discoveryv1.EndpointSlice, error) {
	if l.fails {
		return nil, errors.New("failed to list EndpointSlices")
	}
	return l.slices, nil
}

func (l *fakeEndpointSlicesLister) EndpointSlices(_ string) discoveryv1listers.EndpointSliceNamespaceLister {
	return l
}

func (l *fakeEndpointSlicesLister) Get(name string) (*discoveryv1.EndpointSlice, error) {
	if l.fails {
		return nil, errors.New("failed to get EndpointSlice")
	}
	for _, slice := range l.slices {
		if slice.Name == name {
			return slice, nil
		}
	}
	return nil, errors.New("EndpointSlice not found")
}

func ingressWithVisibility(visibility v1alpha1.IngressVisibility, host string, tls bool) *v1alpha1.Ingress {
	ingress := &v1alpha1.Ingress{
		Spec: v1alpha1.IngressSpec{
			Rules: []v1alpha1.IngressRule{
				{
					Hosts:      []string{host},
					Visibility: visibility,
				},
			},
		},
	}
	if tls {
		ingress.Spec.TLS = []v1alpha1.IngressTLS{
			{
				Hosts: []string{host},
			},
		}
	}
	return ingress
}
