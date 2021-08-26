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

package envoy

import (
	"sort"
	"testing"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gotest.tools/v3/assert"
)

func TestHeadersToAdd(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		out  []*core.HeaderValueOption
	}{{
		name: "nil",
		in:   nil,
		out:  nil,
	}, {
		name: "empty",
		in:   map[string]string{},
		out:  nil,
	}, {
		name: "some",
		in: map[string]string{
			"foo": "bar",
			"baz": "lol",
		},
		out: []*core.HeaderValueOption{{
			Header: &core.HeaderValue{
				Key:   "foo",
				Value: "bar",
			},
			Append: wrapperspb.Bool(false),
		}, {
			Header: &core.HeaderValue{
				Key:   "baz",
				Value: "lol",
			},
			Append: wrapperspb.Bool(false),
		}},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := headersToAdd(test.in)

			sort.Slice(test.out, func(i, j int) bool {
				return test.out[i].Header.Key < test.out[j].Header.Key
			})
			sort.Slice(got, func(i, j int) bool {
				return got[i].Header.Key < got[j].Header.Key
			})

			assert.DeepEqual(t, got, test.out, protocmp.Transform())
		})
	}
}
