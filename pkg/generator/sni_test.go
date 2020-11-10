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
	"sort"
	"testing"

	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/types"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
)

func TestDeduplication(t *testing.T) {
	s1 := types.NamespacedName{
		Namespace: "secret-ns-1",
		Name:      "secret-1",
	}
	s2 := types.NamespacedName{
		Namespace: "secret-ns-2",
		Name:      "secret-2",
	}

	tests := []struct {
		name string
		in   []*envoy.SNIMatch
		out  []*envoy.SNIMatch
	}{{
		name: "distinct matches",
		in: []*envoy.SNIMatch{{
			Hosts:      []string{"foo", "bar"},
			CertSource: s1,
		}, {
			Hosts:      []string{"baz"},
			CertSource: s2,
		}},
		out: []*envoy.SNIMatch{{
			Hosts:      []string{"foo", "bar"},
			CertSource: s1,
		}, {
			Hosts:      []string{"baz"},
			CertSource: s2,
		}},
	}, {
		name: "same secret",
		in: []*envoy.SNIMatch{{
			Hosts:      []string{"foo", "bar"},
			CertSource: s1,
		}, {
			Hosts:      []string{"baz"},
			CertSource: s1,
		}},
		out: []*envoy.SNIMatch{{
			Hosts:      []string{"foo", "bar", "baz"},
			CertSource: s1,
		}},
	}, {
		name: "same secret, one distinct",
		in: []*envoy.SNIMatch{{
			Hosts:      []string{"foo", "bar"},
			CertSource: s1,
		}, {
			Hosts:      []string{"baz"},
			CertSource: s1,
		}, {
			Hosts:      []string{"foo2"},
			CertSource: s2,
		}, {
			Hosts:      []string{"bar2"},
			CertSource: s2,
		}},
		out: []*envoy.SNIMatch{{
			Hosts:      []string{"foo", "bar", "baz"},
			CertSource: s1,
		}, {
			Hosts:      []string{"foo2", "bar2"},
			CertSource: s2,
		}},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matches := sniMatches{}
			for _, match := range test.in {
				matches.consume(match)
			}
			got := matches.list()

			// Sort the lists as we go via maps in the implementatio, so the order is
			// not guaranteed.
			sort.Slice(test.out, func(i, j int) bool {
				return test.out[i].CertSource.String() < test.out[j].CertSource.String()
			})
			sort.Slice(got, func(i, j int) bool {
				return got[i].CertSource.String() < got[j].CertSource.String()
			})

			assert.DeepEqual(t, test.out, got)
		})
	}
}
