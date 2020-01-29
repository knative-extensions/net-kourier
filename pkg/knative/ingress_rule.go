package knative

import (
	"strings"

	"knative.dev/serving/pkg/apis/networking/v1alpha1"
)

// Somehow envoy doesn't match properly gRPC authorities with ports.
// The fix is to include ":*" in the domains.
// This applies both for internal and external domains.
// More info https://github.com/envoyproxy/envoy/issues/886

func ExternalDomains(rule *v1alpha1.IngressRule, localDomainName string) []string {
	var res []string

	for _, host := range rule.Hosts {
		if !strings.Contains(host, localDomainName) {
			res = append(res, host, host+":*")
		}
	}

	return res
}

// InternalDomains returns domains with the following formats:
// 	- sub-route_host
// 	- sub-route_host.namespace
// 	- sub-route_host.namespace.svc
// 	- Each of the previous ones with ":*" appended
func InternalDomains(rule *v1alpha1.IngressRule, localDomainName string) []string {
	var res []string

	for _, host := range rule.Hosts {
		if strings.Contains(host, localDomainName) {
			res = append(res, host, host+":*")

			splits := strings.Split(host, ".")
			domain := splits[0] + "." + splits[1]
			res = append(res, domain, domain+":*")

			domain = splits[0] + "." + splits[1] + ".svc"
			res = append(res, domain, domain+":*")

		}
	}

	return res
}

func RuleIsExternal(rule *v1alpha1.IngressRule, ingressVisibility v1alpha1.IngressVisibility) bool {
	switch rule.Visibility {
	case v1alpha1.IngressVisibilityExternalIP:
		return true
	case v1alpha1.IngressVisibilityClusterLocal:
		return false
	default:
		// If the rule does not have a visibility set, use the one at the ingress level
		// If there is not anything set, Knative defaults to "external"
		return ingressVisibility != v1alpha1.IngressVisibilityClusterLocal
	}
}
