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

package endpointslice

import (
	"testing"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestReadyAddressesFromSlice(t *testing.T) {
	tests := []struct {
		name  string
		slice *discoveryv1.EndpointSlice
		want  []string // nil means we expect nil return, empty slice means we expect empty set
	}{
		{
			name: "IPv4 with ready endpoints",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
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
			},
			want: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name: "IPv4 with mixed ready and not ready",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
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
					{
						Addresses: []string{"10.0.0.3"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(true),
						},
					},
				},
			},
			want: []string{"10.0.0.1", "10.0.0.3"},
		},
		{
			name: "IPv4 with nil ready condition defaults to true",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: nil, // nil defaults to true per K8s semantics
						},
					},
					{
						Addresses: []string{"10.0.0.2"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(true),
						},
					},
				},
			},
			want: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name: "IPv4 with no ready endpoints returns nil",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(false),
						},
					},
					{
						Addresses: []string{"10.0.0.2"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(false),
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "IPv4 with empty endpoints returns nil",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints:   []discoveryv1.Endpoint{},
			},
			want: nil,
		},
		{
			name: "IPv6 slice returns nil",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
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
			},
			want: nil,
		},
		{
			name: "FQDN slice returns nil",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				AddressType: discoveryv1.AddressTypeFQDN,
				Endpoints: []discoveryv1.Endpoint{
					{
						Addresses: []string{"example.com"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(true),
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "IPv4 with multiple addresses per endpoint",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1", "10.0.0.2"},
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(true),
						},
					},
				},
			},
			want: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name: "IPv4 with duplicate addresses across endpoints",
			slice: &discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-slice",
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
						Addresses: []string{"10.0.0.1"}, // duplicate
						Conditions: discoveryv1.EndpointConditions{
							Ready: ptr.To(true),
						},
					},
				},
			},
			want: []string{"10.0.0.1"}, // deduplicated by set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadyAddressesFromSlice(tt.slice)

			// Check if we expected nil
			if tt.want == nil {
				if got != nil {
					t.Errorf("ReadyAddressesFromSlice() = %v, want nil", got)
				}
				return
			}

			// Check if we got nil when we expected addresses
			if got == nil {
				t.Errorf("ReadyAddressesFromSlice() = nil, want %v", tt.want)
				return
			}

			// Check length
			if got.Len() != len(tt.want) {
				t.Errorf("ReadyAddressesFromSlice() returned %d addresses, want %d", got.Len(), len(tt.want))
				return
			}

			// Check each expected address is present
			for _, addr := range tt.want {
				if !got.Has(addr) {
					t.Errorf("ReadyAddressesFromSlice() missing address %s, got %v", addr, got)
				}
			}

			// Check no unexpected addresses
			for addr := range got {
				found := false
				for _, wantAddr := range tt.want {
					if addr == wantAddr {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ReadyAddressesFromSlice() has unexpected address %s", addr)
				}
			}
		})
	}
}
