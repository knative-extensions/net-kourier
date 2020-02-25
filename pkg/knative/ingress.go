package knative

import (
	"knative.dev/pkg/network"
	"knative.dev/pkg/system"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/pkg/client/clientset/versioned"

	"kourier/pkg/config"
)

func MarkIngressReady(knativeClient versioned.Interface, ingress *networkingv1alpha1.Ingress) error {
	var err error
	status := ingress.GetStatus()
	if ingress.GetGeneration() != status.ObservedGeneration || !status.IsReady() {
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
			[]networkingv1alpha1.LoadBalancerIngressStatus{
				{
					DomainInternal: domain,
				},
			},
			[]networkingv1alpha1.LoadBalancerIngressStatus{
				{
					DomainInternal: externalDomain,
				},
			},
			[]networkingv1alpha1.LoadBalancerIngressStatus{
				{
					DomainInternal: internalDomain,
				},
			})
		status.MarkNetworkConfigured()
		status.ObservedGeneration = ingress.GetGeneration()
		ingress.SetStatus(*status)

		_, err = knativeClient.NetworkingV1alpha1().Ingresses(ingress.GetNamespace()).UpdateStatus(ingress)
		return err
	}
	return nil
}

func domainForServiceName(serviceName string) string {
	return serviceName + "." + system.Namespace() + ".svc." + network.GetClusterDomainName()
}
