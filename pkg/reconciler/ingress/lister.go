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
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"go.uber.org/zap"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	discoveryv1listers "k8s.io/client-go/listers/discovery/v1"
	"knative.dev/net-kourier/pkg/endpointslice"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/status"
)

func NewProbeTargetLister(logger *zap.SugaredLogger, endpointSlicesLister discoveryv1listers.EndpointSliceLister) status.ProbeTargetLister {
	return &gatewayPodTargetLister{
		logger:               logger,
		endpointSlicesLister: endpointSlicesLister,
	}
}

type gatewayPodTargetLister struct {
	logger               *zap.SugaredLogger
	endpointSlicesLister discoveryv1listers.EndpointSliceLister
}

func (l *gatewayPodTargetLister) ListProbeTargets(_ context.Context, ing *v1alpha1.Ingress) ([]status.ProbeTarget, error) {
	selector := labels.SelectorFromSet(labels.Set{
		discoveryv1.LabelServiceName: config.InternalServiceName,
	})
	slices, err := l.endpointSlicesLister.EndpointSlices(config.GatewayNamespace()).List(selector)
	if err != nil {
		return nil, fmt.Errorf("failed to list endpointslices for internal service: %w", err)
	}

	// Aggregate ready IPv4 addresses from all slices
	allAddresses := sets.New[string]()
	for _, slice := range slices {
		addresses := endpointslice.ReadyAddressesFromSlice(slice)
		if addresses != nil {
			allAddresses = allAddresses.Union(addresses)
		}
	}

	if allAddresses.Len() == 0 {
		return nil, errors.New("no gateway pods available")
	}

	readyIPs := sets.List(allAddresses)
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
