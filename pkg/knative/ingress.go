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
	"knative.dev/pkg/network"
	"knative.dev/pkg/system"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"

	"knative.dev/net-kourier/pkg/config"
)

func MarkIngressReady(ingress *networkingv1alpha1.Ingress) {
	internalDomain := domainForServiceName(config.InternalServiceName)
	externalDomain := domainForServiceName(config.ExternalServiceName)

	var domain string

	if ingress.Spec.Visibility == networkingv1alpha1.IngressVisibilityClusterLocal {
		domain = internalDomain
	} else {
		domain = externalDomain
	}

	ingress.Status.MarkLoadBalancerReady(
		[]networkingv1alpha1.LoadBalancerIngressStatus{{
			DomainInternal: domain,
		}},
		[]networkingv1alpha1.LoadBalancerIngressStatus{{
			DomainInternal: externalDomain,
		}},
		[]networkingv1alpha1.LoadBalancerIngressStatus{{
			DomainInternal: internalDomain,
		}},
	)
	ingress.Status.MarkNetworkConfigured()
}

func domainForServiceName(serviceName string) string {
	return serviceName + "." + system.Namespace() + ".svc." + network.GetClusterDomainName()
}
