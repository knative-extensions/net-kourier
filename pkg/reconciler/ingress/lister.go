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
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/status"
)

func NewProbeTargetLister(logger *zap.SugaredLogger, endpointsLister corev1listers.EndpointsLister) status.ProbeTargetLister {
	return &gatewayPodTargetLister{
		logger:          logger,
		endpointsLister: endpointsLister,
	}
}

type gatewayPodTargetLister struct {
	logger          *zap.SugaredLogger
	endpointsLister corev1listers.EndpointsLister
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
	return l.getIngressUrls(ing, readyIPs)
}

func (l *gatewayPodTargetLister) getIngressUrls(ing *v1alpha1.Ingress, gatewayIps []string) ([]status.ProbeTarget, error) {
	ips := sets.New(gatewayIps...)

	localIngressTLS := ing.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityClusterLocal)
	externalIngressTLS := ing.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityExternalIP)
	localTLS := len(localIngressTLS) > 0
	externalTLS := len(externalIngressTLS) > 0

	targets := make([]status.ProbeTarget, 0, len(ing.Spec.Rules))
	for _, rule := range ing.Spec.Rules {
		target := status.ProbeTarget{
			PodIPs: ips,
		}

		switch {
		case rule.Visibility == v1alpha1.IngressVisibilityExternalIP && externalTLS:
			target.PodPort = strconv.Itoa(int(config.HTTPSPortProb))
			target.URLs = domainsToURL(rule.Hosts, "https")

		case rule.Visibility == v1alpha1.IngressVisibilityExternalIP && !externalTLS:
			target.PodPort = strconv.Itoa(int(config.HTTPPortProb))
			target.URLs = domainsToURL(rule.Hosts, "http")

		case rule.Visibility == v1alpha1.IngressVisibilityClusterLocal && localTLS:
			target.PodPort = strconv.Itoa(int(config.HTTPSPortLocal))
			target.URLs = domainsToURL(rule.Hosts, "https")

		case rule.Visibility == v1alpha1.IngressVisibilityClusterLocal && !localTLS:
			target.PodPort = strconv.Itoa(int(config.HTTPPortLocal))
			target.URLs = domainsToURL(rule.Hosts, "http")
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
