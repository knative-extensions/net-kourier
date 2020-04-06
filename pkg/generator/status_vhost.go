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
	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/envoy"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
)

// Generates an internal virtual host that signals that the Envoy instance has
// been configured, this endpoint is used by the kubernetes readiness probe.
func statusVHost() route.VirtualHost {
	return envoy.NewVirtualHost(
		config.InternalKourierDomain,
		[]string{config.InternalKourierDomain},
		[]*route.Route{statusRoute()},
	)
}

func statusRoute() *route.Route {
	return envoy.NewRouteStatusOK(
		config.InternalKourierDomain,
		config.InternalKourierPath,
	)
}
