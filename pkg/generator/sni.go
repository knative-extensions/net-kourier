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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
)

// dedupedSNIMatch is a helper type that keeps the set of all added hosts as a set to
// optimize lookup times for hosts to add.
type dedupedSNIMatch struct {
	sniMatch *envoy.SNIMatch
	hosts    sets.String
}

// sniMatches is a collection of deduplicated sni matches that can be used to deduplicate
// an existing list of sniMatches to avoid allocating a lot of configuration memory for
// tls configurations that are essentially equal.
// SNIMatches are deduplicated and collapsed by collapsing the list of hosts of all
// matches that have the same certificate source (i.e. reference the same Secret).
type sniMatches map[types.NamespacedName]*dedupedSNIMatch

func (s sniMatches) consume(match *envoy.SNIMatch) {
	state := s[match.CertSource]
	if state == nil {
		state = &dedupedSNIMatch{
			sniMatch: match,
			hosts:    sets.NewString(match.Hosts...),
		}
		s[match.CertSource] = state
		return
	}

	for _, host := range match.Hosts {
		if !state.hosts.Has(host) {
			state.sniMatch.Hosts = append(state.sniMatch.Hosts, host)
			state.hosts.Insert(host)
		}
	}
}

// list returns the deduplicated and collapsed list of SNIMatches.
func (s sniMatches) list() []*envoy.SNIMatch {
	if len(s) == 0 {
		return nil
	}

	matches := make([]*envoy.SNIMatch, 0, len(s))
	for _, match := range s {
		matches = append(matches, match.sniMatch)
	}
	return matches
}
