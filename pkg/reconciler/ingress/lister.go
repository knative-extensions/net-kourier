/*
Copyright 2020 The Knative Authors.

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

package ingress

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/net-kourier/pkg/config"
	ingressconfig "knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/status"
)

func NewProbeTargetLister(logger *zap.SugaredLogger, endpointsLister corev1listers.EndpointsLister, namespaceLister corev1listers.NamespaceLister) status.ProbeTargetLister {
	return &gatewayPodTargetLister{
		logger:          logger,
		endpointsLister: endpointsLister,
		namespaceLister: namespaceLister,
	}
}

type gatewayPodTargetLister struct {
	logger          *zap.SugaredLogger
	endpointsLister corev1listers.EndpointsLister
	namespaceLister corev1listers.NamespaceLister
}

func (l *gatewayPodTargetLister) ListProbeTargets(ctx context.Context, ing *v1alpha1.Ingress) ([]status.ProbeTarget, error) {
	eps, err := l.endpointsLister.Endpoints(config.GatewayNamespace()).Get(config.InternalServiceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get internal service: %w", err)
	}

	var readyIPs []string
	for _, sub := range eps.Subsets {
		for _, address := range sub.Addresses {
			readyIPs = append(readyIPs, address.IP)
		}
	}
	if len(readyIPs) == 0 {
		return nil, fmt.Errorf("no gateway pods available")
	}
	return l.getIngressUrls(ctx, ing, readyIPs)
}

func (l *gatewayPodTargetLister) getIngressUrls(ctx context.Context, ing *v1alpha1.Ingress, gatewayIps []string) ([]status.ProbeTarget, error) {
	ips := sets.NewString(gatewayIps...)

	targets := make([]status.ProbeTarget, 0, len(ing.Spec.Rules))
	for _, rule := range ing.Spec.Rules {
		var target status.ProbeTarget

		domains := rule.Hosts
		scheme := "http"

		if rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			target = status.ProbeTarget{
				PodIPs: ips,
			}
			if len(ing.Spec.TLS) != 0 {
				target.PodPort = strconv.Itoa(int(config.HTTPSPortProb))
				target.URLs = domainsToURL(domains, "https")
			} else {
				target.PodPort = strconv.Itoa(int(config.HTTPPortProb))
				target.URLs = domainsToURL(domains, scheme)
			}
		} else {
			podPort := strconv.Itoa(int(config.HTTPPortInternal))

			if ingressconfig.FromContextOrDefaults(ctx).Kourier.TrafficIsolation == config.IsolationIngressPort {
				ns, err := l.namespaceLister.Get(ing.Namespace)
				if err != nil {
					return nil, fmt.Errorf("failed to get the ingress namespace: %w", err)
				}

				if ns.Annotations != nil {
					if value, ok := ns.Annotations[config.ListenerPortAnnotationKey]; ok {
						podPort = value
					}
				}
			}

			target = status.ProbeTarget{
				PodIPs:  ips,
				PodPort: podPort,
				URLs:    domainsToURL(domains, scheme),
			}
		}

		targets = append(targets, target)

	}
	return targets, nil
}

func domainsToURL(domains []string, scheme string) []*url.URL {
	urls := make([]*url.URL, 0, len(domains))
	for _, domain := range domains {
		url := &url.URL{
			Scheme: scheme,
			Host:   domain,
			Path:   "/",
		}
		urls = append(urls, url)
	}
	return urls
}
