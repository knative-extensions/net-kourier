package knative

import (
	"fmt"

	"knative.dev/pkg/system"

	"knative.dev/serving/pkg/client/clientset/versioned"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/apis/networking"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
)

const (
	internalServiceName     = "kourier-internal"
	externalServiceName     = "kourier-external"
	kourierIngressClassName = "kourier.ingress.networking.knative.dev"
)

func Services(knativeClient versioned.Interface, namespace string) (*v1alpha1.ServiceList, error) {
	return knativeClient.ServingV1alpha1().Services(namespace).List(v1.ListOptions{})
}

func MarkIngressReady(knativeClient versioned.Interface, ingress networkingv1alpha1.IngressAccessor) error {
	// TODO: Improve. Currently once we go trough the generation of the envoy cache, we mark the objects as Ready,
	//  but that is not exactly true, it can take a while until envoy exposes the routes. Is there a way to get a "callback" from envoy?
	var err error
	status := ingress.GetStatus()
	if ingress.GetGeneration() != status.ObservedGeneration || !ingress.GetStatus().IsReady() {

		internalDomain := internalServiceName + "." + system.Namespace() + ".svc.cluster.local"
		externalDomain := externalServiceName + "." + system.Namespace() + ".svc.cluster.local"

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

		// Handle both types of ingresses
		switch ingress.(type) {
		case *networkingv1alpha1.Ingress:
			in := ingress.(*networkingv1alpha1.Ingress)
			_, err = knativeClient.NetworkingV1alpha1().Ingresses(ingress.GetNamespace()).UpdateStatus(in)
			return err
		default:
			return fmt.Errorf("can't update object, not Ingress")
		}
	}
	return nil
}

func FilterByIngressClass(ingresses []*networkingv1alpha1.Ingress) []*networkingv1alpha1.Ingress {
	var res []*networkingv1alpha1.Ingress

	for _, ingress := range ingresses {
		ingressClass := ingressClass(ingress)

		if ingressClass == kourierIngressClassName {
			res = append(res, ingress)
		}
	}

	return res
}

func ingressClass(ingress *networkingv1alpha1.Ingress) string {
	return ingress.GetObjectMeta().GetAnnotations()[networking.IngressClassAnnotationKey]
}
