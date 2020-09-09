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

package knative

import (
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

// Domains returns domains.
//
// For example, external domains returns domains with the following formats:
// 	- sub-route_host.namespace.example.com
// 	- sub-route_host.namespace.example.com:*
//
// Somehow envoy doesn't match properly gRPC authorities with ports.
// The fix is to include ":*" in the domains.
// This applies both for internal and external domains.
// More info https://github.com/envoyproxy/envoy/issues/886
//
func Domains(rule v1alpha1.IngressRule) []string {
	domains := make([]string, 0, 2*len(rule.Hosts))
	for _, host := range rule.Hosts {
		domains = append(domains, host, host+":*")
	}
	return domains
}

func RuleIsExternal(rule v1alpha1.IngressRule) bool {
	return rule.Visibility == v1alpha1.IngressVisibilityExternalIP
}
