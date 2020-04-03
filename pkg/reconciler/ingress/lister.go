/*
Copyright 2019 The Knative Authors.

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
	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/knative"
	"knative.dev/pkg/system"
	"net/url"
	"strconv"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/pkg/network/status"
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

	eps, err := l.endpointsLister.Endpoints(system.Namespace()).Get(config.InternalServiceName)
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
	var targets []status.ProbeTarget
	var urls []*url.URL
	schema := "http"

	for _, ip := range gatewayIps {
		target := status.ProbeTarget{
			PodPort: strconv.Itoa(int(config.HTTPPortInternal)),
			Port:    strconv.Itoa(int(config.HTTPPortInternal)),
			PodIPs:  sets.String{ip: sets.Empty{}},
		}

		for _, rule := range ing.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				for _, host := range rule.Hosts {
					targetURL, _ := url.ParseRequestURI(schema + "://" + host + path.Path)
					urls = append(urls, targetURL)
				}
			}
			target.URLs = urls

			if len(ing.Spec.TLS) != 0 {
				schema = "https"
				target.PodPort = strconv.Itoa(int(config.HTTPSPortExternal))
				target.Port = target.PodPort
			} else {
				if knative.RuleIsExternal(rule, ing.Spec.Visibility) {
					target.PodPort = strconv.Itoa(int(config.HTTPPortExternal))
					target.Port = target.PodPort
				}
			}
			l.logger.Debugf("Adding target URLs: %s for ingress: %s in namespace: %s using port: %s for gatewayIP: %s", target.URLs, ing.Name, ing.Namespace, target.Port, ip)
			targets = append(targets, target)
		}
	}
	return nil, nil
}
