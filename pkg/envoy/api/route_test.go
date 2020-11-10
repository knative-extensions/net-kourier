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
	"gotest.tools/assert"
)

func TestNewRouteHeaderMatch(t *testing.T) {
	name := "testRoute_12345"
	path := "/my_route"
	headerMatch := []*route.HeaderMatcher{{
		Name: "myHeader",
		HeaderMatchSpecifier: &route.HeaderMatcher_ExactMatch{
			ExactMatch: "strict",
		},
	}}

	r := NewRoute(name, headerMatch, path, nil, 0, nil, "")
	assert.Equal(t, r.Match.Headers[0].Name, "myHeader")
	assert.Equal(t, r.Match.Headers[0].GetExactMatch(), "strict")
}

func TestNewRouteHostRewrite(t *testing.T) {
	name := "testRoute_12345"
	path := "/my_route"

	r := NewRoute(name, nil, path, nil, 0, nil, "test.host")
	assert.Equal(t, r.Action.(*route.Route_Route).Route.GetHostRewrite(), "test.host")
}
