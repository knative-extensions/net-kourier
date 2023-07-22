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
	"testing"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gotest.tools/v3/assert"
)

func TestNewWeightedCluster(t *testing.T) {
	got := NewWeightedCluster("test", 50, map[string]string{
		"foo": "bar",
	})
	want := &route.WeightedCluster_ClusterWeight{
		Name:   "test",
		Weight: wrapperspb.UInt32(50),
		RequestHeadersToAdd: []*core.HeaderValueOption{{
			Header: &core.HeaderValue{
				Key:   "foo",
				Value: "bar",
			},
			AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		}},
	}

	assert.DeepEqual(t, got, want, protocmp.Transform())
}
