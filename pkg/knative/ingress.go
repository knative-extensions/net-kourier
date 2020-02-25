package knative

import (
	"knative.dev/pkg/network"
	"knative.dev/pkg/system"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"

	"kourier/pkg/config"
)

func MarkIngressReady(ingress *networkingv1alpha1.Ingress) {
	status := ingress.GetStatus()
	internalDomain := domainForServiceName(config.InternalServiceName)
	externalDomain := domainForServiceName(config.ExternalServiceName)

	status.InitializeConditions()
	var domain string

	if ingress.GetSpec().Visibility == networkingv1alpha1.IngressVisibilityClusterLocal {
		domain = internalDomain
	} else {
		domain = externalDomain
	}

	status.MarkLoadBalancerReady(
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
	status.MarkNetworkConfigured()
	status.ObservedGeneration = ingress.GetGeneration()
	ingress.SetStatus(*status)
}

func domainForServiceName(serviceName string) string {
	return serviceName + "." + system.Namespace() + ".svc." + network.GetClusterDomainName()
}
