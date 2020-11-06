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

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"gotest.tools/assert"
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

	assert.DeepEqual(t, got, want)
}

func TestVirtualHostWithExtAuthz(t *testing.T) {
	name := "test"
	domains := []string{"foo", "bar"}
	routes := []*route.Route{{Name: "baz"}}

	got := NewVirtualHostWithExtAuthz(name, nil, domains, routes)
	want := &route.VirtualHost{
		Name:    name,
		Domains: domains,
		Routes:  routes,
	}

	assert.Equal(t, got.Name, want.Name)
	assert.DeepEqual(t, got.Domains, want.Domains)
	assert.DeepEqual(t, got.Routes, want.Routes)
	assert.Assert(t, got.TypedPerFilterConfig[wellknown.HTTPExternalAuthorization] != nil)
}
