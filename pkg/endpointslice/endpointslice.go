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
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ReadyAddressesFromSlice extracts ready IPv4 addresses from an EndpointSlice.
// Returns nil if the slice is non-IPv4 or has no ready endpoints.
//
// Future consideration: A more sophisticated implementation could check Serving and
// Terminating conditions to support graceful draining during rolling updates, similar
// to Istio, Traefik, and MetalLB implementations.
func ReadyAddressesFromSlice(slice *discoveryv1.EndpointSlice) sets.Set[string] {
	if slice.AddressType != discoveryv1.AddressTypeIPv4 {
		return nil
	}

	ready := sets.New[string]()
	for _, ep := range slice.Endpoints {
		// Only include endpoints where Ready is nil or true, following Kubernetes semantics
		// where nil Ready defaults to true.
		if ep.Conditions.Ready == nil || *ep.Conditions.Ready {
			for _, addr := range ep.Addresses {
				ready.Insert(addr)
			}
		}
	}

	if ready.Len() == 0 {
		return nil
	}

	return ready
}
