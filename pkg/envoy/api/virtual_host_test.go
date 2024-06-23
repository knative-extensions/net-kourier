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

	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gotest.tools/v3/assert"
)

func TestVirtualHost(t *testing.T) {
	name := "test"
	domains := []string{"foo", "bar"}
	routes := []*route.Route{{Name: "baz"}}

	got := NewVirtualHost(name, domains, routes)
	want := &route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
	}

	assert.DeepEqual(t, got, want, protocmp.Transform())
}

func TestVirtualHostWithExtAuthz(t *testing.T) {
	name := "test"
	domains := []string{"foo", "bar"}
	routes := []*route.Route{{Name: "baz"}}

	got := NewVirtualHost(name, domains, routes, WithExtAuthz(nil))
	want := &route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
	}

	assert.Equal(t, got.Name, want.Name)
	assert.DeepEqual(t, got.Domains, want.Domains)
	assert.DeepEqual(t, got.Routes, want.Routes, protocmp.Transform())
	assert.Assert(t, got.TypedPerFilterConfig[wellknown.HTTPExternalAuthorization] != nil)
}

func TestVirtualHostWithRetryOnTransientUpstreamFailure(t *testing.T) {
	name := "test"
	domains := []string{"foo", "bar"}
	routes := []*route.Route{{Name: "baz"}}

	got := NewVirtualHost(name, domains, routes, WithRetryOnTransientUpstreamFailure())
	want := &route.VirtualHost{
		Name:        name,
		Domains:     domains,
		Routes:      routes,
		RetryPolicy: &route.RetryPolicy{
			RetryOn:    "reset,connect-failure",
			NumRetries: wrapperspb.UInt32(1),
		},
	}

	assert.Equal(t, got.Name, want.Name)
	assert.DeepEqual(t, got.Domains, want.Domains)
	assert.DeepEqual(t, got.Routes, want.Routes, protocmp.Transform())
	assert.DeepEqual(t, got.RetryPolicy, want.RetryPolicy, protocmp.Transform())
}
