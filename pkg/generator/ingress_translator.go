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

package generator

import (
	"context"
	"fmt"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/net-kourier/pkg/envoy"
	"knative.dev/net-kourier/pkg/knative"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/tracker"
)

type translatedIngress struct {
	ingressName          string
	ingressNamespace     string
	sniMatches           []*envoy.SNIMatch
	clusters             []*v2.Cluster
	externalVirtualHosts []*route.VirtualHost
	internalVirtualHosts []*route.VirtualHost
}

type IngressTranslator struct {
	kubeclient      kubeclient.Interface
	endpointsGetter func(ns, name string) (*corev1.Endpoints, error)
	serviceGetter   func(ns, name string) (*corev1.Service, error)
	tracker         tracker.Interface
}

func NewIngressTranslator(
	kubeclient kubeclient.Interface,
	endpointsGetter func(ns, name string) (*corev1.Endpoints, error),
	serviceGetter func(ns, name string) (*corev1.Service, error),
	tracker tracker.Interface) IngressTranslator {
	return IngressTranslator{
		kubeclient:      kubeclient,
		endpointsGetter: endpointsGetter,
		serviceGetter:   serviceGetter,
		tracker:         tracker,
	}
}

func (translator *IngressTranslator) translateIngress(ctx context.Context, ingress *v1alpha1.Ingress, extAuthzEnabled bool) (*translatedIngress, error) {
	logger := logging.FromContext(ctx)

	sniMatches := make([]*envoy.SNIMatch, 0, len(ingress.Spec.TLS))
	for _, ingressTLS := range ingress.Spec.TLS {
		sniMatch, err := sniMatchFromIngressTLS(ctx, ingressTLS, translator.kubeclient)
		if err != nil {
			// We need to propagate this error to the reconciler so the current
			// event can be retried. This error might be caused because the
			// secrets referenced in the TLS section of the spec do not exist
			// yet. That's expected when auto TLS is configured.
			// See the "TestPerKsvcCert_localCA" test in Knative Serving. It's a
			// test that fails if this error is not propagated:
			// https://github.com/knative/serving/blob/571e4db2392839082c559870ea8d4b72ef61e59d/test/e2e/autotls/auto_tls_test.go#L68
			return nil, fmt.Errorf("failed to get sniMatch: %w", err)
		}
		sniMatches = append(sniMatches, sniMatch)
	}

	internalHosts := make([]*route.VirtualHost, 0, len(ingress.Spec.Rules))
	externalHosts := make([]*route.VirtualHost, 0, len(ingress.Spec.Rules))
	clusters := make([]*v2.Cluster, 0, len(ingress.Spec.Rules))

	for _, rule := range ingress.Spec.Rules {
		routes := make([]*route.Route, 0, len(rule.HTTP.Paths))
		for _, httpPath := range rule.HTTP.Paths {
			// Default the path to "/" if none is passed.
			path := httpPath.Path
			if path == "" {
				path = "/"
			}

			wrs := make([]*route.WeightedCluster_ClusterWeight, 0, len(httpPath.Splits))
			for _, split := range httpPath.Splits {
				if err := trackService(translator.tracker, split.ServiceName, ingress); err != nil {
					return nil, err
				}

				endpoints, err := translator.endpointsGetter(split.ServiceNamespace, split.ServiceName)
				if apierrors.IsNotFound(err) {
					logger.Warnf("Endpoints '%s/%s' not yet created", split.ServiceNamespace, split.ServiceName)
					// TODO(markusthoemmes): Find out if we should actually `continue` here.
					return nil, nil
				} else if err != nil {
					return nil, fmt.Errorf("failed to fetch endpoints '%s/%s': %w", split.ServiceNamespace, split.ServiceName, err)
				}

				service, err := translator.serviceGetter(split.ServiceNamespace, split.ServiceName)
				if apierrors.IsNotFound(err) {
					logger.Warnf("Service '%s/%s' not yet created", split.ServiceNamespace, split.ServiceName)
					// TODO(markusthoemmes): Find out if we should actually `continue` here.
					return nil, nil
				} else if err != nil {
					return nil, fmt.Errorf("failed to fetch service '%s/%s': %w", split.ServiceNamespace, split.ServiceName, err)
				}

				// Match the ingress' port with a port on the Service to find the target.
				// Also find out if the target supports HTTP2.
				var (
					targetPort int32
					http2      bool
				)
				for _, port := range service.Spec.Ports {
					if port.Port == split.ServicePort.IntVal || port.Name == split.ServicePort.StrVal {
						targetPort = port.TargetPort.IntVal
						http2 = port.Name == "http2" || port.Name == "h2c"
					}
				}

				publicLbEndpoints := lbEndpointsForKubeEndpoints(endpoints, targetPort)
				connectTimeout := 5 * time.Second
				cluster := envoy.NewCluster(split.ServiceName+path, connectTimeout, publicLbEndpoints, http2, v2.Cluster_STATIC)
				clusters = append(clusters, cluster)

				weightedCluster := envoy.NewWeightedCluster(split.ServiceName+path, uint32(split.Percent), split.AppendHeaders)
				wrs = append(wrs, weightedCluster)
			}

			if len(wrs) != 0 {
				// Note: routeName uses the non-defaulted path.
				routeName := ingress.Name + "_" + ingress.Namespace + "_" + httpPath.Path
				routes = append(routes, envoy.NewRoute(
					routeName, matchHeadersFromHTTPPath(httpPath), path, wrs, 0, httpPath.AppendHeaders))
			}
		}

		if len(routes) == 0 {
			// Return nothing if there are not routes to generate.
			return nil, nil
		}

		domains := knative.Domains(rule)
		var virtualHost route.VirtualHost
		if extAuthzEnabled {
			contextExtensions := kmeta.UnionMaps(map[string]string{
				"client":     "kourier",
				"visibility": string(rule.Visibility),
			}, ingress.GetLabels())
			virtualHost = envoy.NewVirtualHostWithExtAuthz(ingress.Name, contextExtensions, domains, routes)
		} else {
			virtualHost = envoy.NewVirtualHost(ingress.Name, domains, routes)
		}

		internalHosts = append(internalHosts, &virtualHost)
		if knative.RuleIsExternal(rule) {
			externalHosts = append(externalHosts, &virtualHost)
		}
	}

	return &translatedIngress{
		ingressName:          ingress.Name,
		ingressNamespace:     ingress.Namespace,
		sniMatches:           sniMatches,
		clusters:             clusters,
		externalVirtualHosts: externalHosts,
		internalVirtualHosts: internalHosts,
	}, nil
}

func trackService(t tracker.Interface, svcName string, ingress *v1alpha1.Ingress) error {
	if err := t.TrackReference(tracker.Reference{
		Kind:       "Service",
		APIVersion: "v1",
		Namespace:  ingress.Namespace,
		Name:       svcName,
	}, ingress); err != nil {
		return fmt.Errorf("could not track service reference: %w", err)
	}

	if err := t.TrackReference(tracker.Reference{
		Kind:       "Endpoints",
		APIVersion: "v1",
		Namespace:  ingress.Namespace,
		Name:       svcName,
	}, ingress); err != nil {
		return fmt.Errorf("could not track endpoints reference: %w", err)
	}
	return nil
}

func lbEndpointsForKubeEndpoints(kubeEndpoints *corev1.Endpoints, targetPort int32) []*endpoint.LbEndpoint {
	var readyAddressCount int
	for _, subset := range kubeEndpoints.Subsets {
		readyAddressCount += len(subset.Addresses)
	}

	if readyAddressCount == 0 {
		return nil
	}

	eps := make([]*endpoint.LbEndpoint, 0, readyAddressCount)
	for _, subset := range kubeEndpoints.Subsets {
		for _, address := range subset.Addresses {
			eps = append(eps, envoy.NewLBEndpoint(address.IP, uint32(targetPort)))
		}
	}

	return eps
}

func matchHeadersFromHTTPPath(httpPath v1alpha1.HTTPIngressPath) []*route.HeaderMatcher {
	matchHeaders := make([]*route.HeaderMatcher, 0, len(httpPath.Headers))

	for header, matchType := range httpPath.Headers {
		matchHeader := &route.HeaderMatcher{
			Name: header,
		}
		if matchType.Exact != "" {
			matchHeader.HeaderMatchSpecifier = &route.HeaderMatcher_ExactMatch{
				ExactMatch: matchType.Exact,
			}
		}
		matchHeaders = append(matchHeaders, matchHeader)
	}
	return matchHeaders
}

func sniMatchFromIngressTLS(ctx context.Context, ingressTLS v1alpha1.IngressTLS, kubeClient kubeclient.Interface) (*envoy.SNIMatch, error) {
	certChain, privateKey, err := sslCreds(
		ctx, kubeClient, ingressTLS.SecretNamespace, ingressTLS.SecretName,
	)

	if err != nil {
		return nil, err
	}

	sniMatch := envoy.NewSNIMatch(ingressTLS.Hosts, certChain, privateKey)
	return &sniMatch, nil
}
