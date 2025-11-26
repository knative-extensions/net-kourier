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

package generator

import (
	"testing"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestLbEndpointsForKubeEndpointSlices(t *testing.T) {
	tests := []struct {
		name       string
		slices     []*discoveryv1.EndpointSlice
		targetPort int32
		want       int
	}{
		{
			name: "single slice with ready endpoints",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc-abc",
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelServiceName: "test-svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.0.0.1"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
						},
						{
							Addresses: []string{"10.0.0.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Port: ptr.To(int32(8080)),
						},
					},
				},
			},
			targetPort: 8080,
			want:       2,
		},
		{
			name: "multiple slices aggregation",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc-abc",
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelServiceName: "test-svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.0.0.1"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Port: ptr.To(int32(8080)),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc-xyz",
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelServiceName: "test-svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.0.0.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Port: ptr.To(int32(8080)),
						},
					},
				},
			},
			targetPort: 8080,
			want:       2,
		},
		{
			name: "filter not ready endpoints",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc-abc",
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelServiceName: "test-svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.0.0.1"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
						},
						{
							Addresses: []string{"10.0.0.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(false),
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Port: ptr.To(int32(8080)),
						},
					},
				},
			},
			targetPort: 8080,
			want:       1,
		},
		{
			name: "filter IPv6 and FQDN addressing",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc-ipv4",
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelServiceName: "test-svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.0.0.1"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Port: ptr.To(int32(8080)),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc-ipv6",
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelServiceName: "test-svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv6,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"2001:db8::1"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(true),
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Port: ptr.To(int32(8080)),
						},
					},
				},
			},
			targetPort: 8080,
			want:       1, // only IPv4
		},
		{
			name:       "empty slices",
			slices:     []*discoveryv1.EndpointSlice{},
			targetPort: 8080,
			want:       0,
		},
		{
			name: "no ready endpoints",
			slices: []*discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc-abc",
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelServiceName: "test-svc",
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.0.0.1"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: ptr.To(false),
							},
						},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Port: ptr.To(int32(8080)),
						},
					},
				},
			},
			targetPort: 8080,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lbEndpointsForKubeEndpointSlices(tt.slices, tt.targetPort)
			if len(got) != tt.want {
				t.Errorf("lbEndpointsForKubeEndpointSlices() returned %d endpoints, want %d", len(got), tt.want)
			}
		})
	}
}
