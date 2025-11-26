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
	"sort"

	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/net-kourier/pkg/endpointslice"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
)

// lbEndpointsForKubeEndpointSlices converts Kubernetes EndpointSlice resources
// to Envoy LbEndpoints. It aggregates endpoints from multiple slices and filters
// for IPv4 addressing mode and ready endpoints only.
func lbEndpointsForKubeEndpointSlices(slices []*discoveryv1.EndpointSlice, targetPort int32) []*endpoint.LbEndpoint {
	// Aggregate all ready IPv4 addresses from all slices
	allAddresses := sets.New[string]()
	for _, slice := range slices {
		addresses := endpointslice.ReadyAddressesFromSlice(slice)
		if addresses != nil {
			allAddresses = allAddresses.Union(addresses)
		}
	}

	if allAddresses.Len() == 0 {
		return nil
	}

	// Convert addresses to LbEndpoints
	// Sort addresses for consistent ordering in tests and debugging
	sortedAddresses := sets.List(allAddresses)
	sort.Strings(sortedAddresses)

	eps := make([]*endpoint.LbEndpoint, 0, len(sortedAddresses))
	for _, addr := range sortedAddresses {
		eps = append(eps, envoy.NewLBEndpoint(addr, uint32(targetPort))) //#nosec G115
	}

	return eps
}
