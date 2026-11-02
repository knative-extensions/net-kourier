/*
Copyright 2026 The Knative Authors

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
	"context"
	"regexp"
	"testing"

	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"gotest.tools/v3/assert"
	"k8s.io/client-go/kubernetes/fake"

	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
)

func TestStatsListenerInSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		addresses []string
	}{
		{name: "default IPv4"},
		{name: "IPv6", addresses: []string{"::"}},
		{name: "dual stack", addresses: []string{"0.0.0.0", "::"}},
		{name: "IPv6 first", addresses: []string{"::", "0.0.0.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.FromContextOrDefaults(context.Background())
			if tc.addresses != nil {
				cfg.Kourier.ListenIPAddresses = tc.addresses
			}
			// These application settings must not leak into the admin proxy.
			cfg.Kourier.EnableProxyProtocol = true
			cfg.Kourier.EnableServiceAccessLogging = true
			cfg.Kourier.ExternalAuthz.Enabled = true
			ctx := config.ToContext(context.Background(), cfg)
			caches, err := NewCaches(ctx, &fake.Clientset{})
			assert.NilError(t, err)
			snapshot, err := caches.ToEnvoySnapshot(ctx)
			assert.NilError(t, err)
			l, ok := snapshot.GetResources(resource.ListenerType)["stats_listener"].(*listener.Listener)
			assert.Assert(t, ok, "stats listener must exist even without ingresses")
			assert.NilError(t, l.ValidateAll())
			addresses := make([]string, 0, 1+len(l.AdditionalAddresses))
			addresses = append(addresses, l.Address.GetSocketAddress().Address)
			assert.Equal(t, l.Address.GetSocketAddress().GetPortValue(), uint32(9000))
			for _, address := range l.AdditionalAddresses {
				addresses = append(addresses, address.Address.GetSocketAddress().Address)
				assert.Equal(t, address.Address.GetSocketAddress().GetPortValue(), uint32(9000))
			}
			assert.DeepEqual(t, addresses, cfg.Kourier.ListenIPAddresses)
			assert.Equal(t, len(l.ListenerFilters), 0)
			manager := &hcm.HttpConnectionManager{}
			assert.NilError(t, l.FilterChains[0].Filters[0].GetTypedConfig().UnmarshalTo(manager))
			assert.NilError(t, manager.ValidateAll())
			assert.Equal(t, manager.StatPrefix, "stats_server")
			assert.Equal(t, len(manager.HttpFilters), 1)
			assert.Equal(t, manager.HttpFilters[0].Name, wellknown.Router)
			assert.Equal(t, len(manager.AccessLog), 0)
			assert.Assert(t, manager.Tracing == nil)
			assert.Assert(t, manager.GetRds() == nil)
		})
	}
}

func TestStatsListenerRoutes(t *testing.T) {
	l, err := statsListener([]string{"0.0.0.0"})
	assert.NilError(t, err)
	manager := &hcm.HttpConnectionManager{}
	assert.NilError(t, l.FilterChains[0].Filters[0].GetTypedConfig().UnmarshalTo(manager))
	vhosts := manager.GetRouteConfig().VirtualHosts
	assert.Equal(t, len(vhosts), 1)
	assert.Equal(t, vhosts[0].Name, "admin_interface")
	assert.DeepEqual(t, vhosts[0].Domains, []string{"*"})
	assert.Equal(t, len(vhosts[0].Routes), 2)

	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/", true},
		{"GET", "/certs", true},
		{"GET", "/stats", true},
		{"GET", "/stats/prometheus", true},
		{"GET", "/server_info", true},
		{"GET", "/clusters", true},
		{"GET", "/listeners", true},
		{"GET", "/ready", true},
		{"POST", "/drain_listeners", true},
		{"GET", "/drain_listeners", false},
		{"POST", "/stats", false},
		{"GET", "/config_dump", false},
		{"POST", "/quitquitquit", false},
		{"POST", "/logging", false},
		{"GET", "/stats/prometheus/extra", false},
		{"GET", "/prefix/stats", false},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			allowed := false
			for _, r := range vhosts[0].Routes {
				assert.Equal(t, r.GetRoute().GetCluster(), "service_stats")
				assert.Equal(t, len(r.Match.Headers), 1)
				assert.Equal(t, r.Match.Headers[0].Name, ":method")
				// Envoy safe_regex matches the entire path, not a substring.
				path := regexp.MustCompile("^(?:" + r.Match.GetSafeRegex().Regex + ")$")
				if path.MatchString(tc.path) && r.Match.Headers[0].GetStringMatch().GetExact() == tc.method {
					allowed = true
				}
			}
			assert.Equal(t, allowed, tc.allowed)
		})
	}
}

func TestStatsListenerRequiresAddress(t *testing.T) {
	_, err := statsListener(nil)
	assert.Assert(t, err != nil)
}
