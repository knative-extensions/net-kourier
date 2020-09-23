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

	"knative.dev/net-kourier/pkg/envoy"
	"knative.dev/net-kourier/pkg/knative"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"go.uber.org/zap"
	kubev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/pkg/tracker"
)

type IngressTranslator struct {
	kubeclient      kubeclient.Interface
	endpointsLister corev1listers.EndpointsLister
	tracker         tracker.Interface
	logger          *zap.SugaredLogger
}

type translatedIngress struct {
	ingressName          string
	ingressNamespace     string
	sniMatches           []*envoy.SNIMatch
	routes               []*route.Route
	clusters             []*v2.Cluster
	externalVirtualHosts []*route.VirtualHost
	internalVirtualHosts []*route.VirtualHost
}

func NewIngressTranslator(kubeclient kubeclient.Interface, endpointsLister corev1listers.EndpointsLister, tracker tracker.Interface,
	logger *zap.SugaredLogger) IngressTranslator {
	return IngressTranslator{
		kubeclient:      kubeclient,
		endpointsLister: endpointsLister,
		tracker:         tracker,
		logger:          logger,
	}
}

func (translator *IngressTranslator) translateIngress(ctx context.Context, ingress *v1alpha1.Ingress, extAuthzEnabled bool) (*translatedIngress, error) {
	res := &translatedIngress{
		ingressName:      ingress.Name,
		ingressNamespace: ingress.Namespace,
	}

	for _, ingressTLS := range ingress.Spec.TLS {
		sniMatch, err := sniMatchFromIngressTLS(ctx, ingressTLS, translator.kubeclient)

		if err != nil {
			translator.logger.Errorf("%s", err)

			// We need to propagate this error to the reconciler so the current
			// event can be retried. This error might be caused because the
			// secrets referenced in the TLS section of the spec do not exist
			// yet. That's expected when auto TLS is configured.
			// See the "TestPerKsvcCert_localCA" test in Knative Serving. It's a
			// test that fails if this error is not propagated:
			// https://github.com/knative/serving/blob/571e4db2392839082c559870ea8d4b72ef61e59d/test/e2e/autotls/auto_tls_test.go#L68
			return nil, err
		}
		res.sniMatches = append(res.sniMatches, sniMatch)
	}

	for _, rule := range ingress.Spec.Rules {

		var ruleRoute []*route.Route

		for _, httpPath := range rule.HTTP.Paths {

			path := "/"
			if httpPath.Path != "" {
				path = httpPath.Path
			}

			var wrs []*route.WeightedCluster_ClusterWeight

			for _, split := range httpPath.Splits {
				if err := trackService(translator.tracker, split.ServiceName, ingress); err != nil {
					return nil, err
				}

				endpoints, err := translator.endpointsLister.Endpoints(split.ServiceNamespace).Get(split.ServiceName)
				if apierrors.IsNotFound(err) {
					translator.logger.Warnf("Endpoints '%s/%s' not yet created", split.ServiceNamespace, split.ServiceName)
					return nil, nil
				} else if err != nil {
					return nil, fmt.Errorf("failed to fetch endpoints '%s/%s': %w", split.ServiceNamespace, split.ServiceName, err)
				}

				service, err := translator.kubeclient.CoreV1().Services(split.ServiceNamespace).Get(ctx, split.ServiceName, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					translator.logger.Warnf("Service '%s/%s' not yet created", split.ServiceNamespace, split.ServiceName)
					return nil, nil
				} else if err != nil {
					return nil, fmt.Errorf("failed to fetch service '%s/%s': %w", split.ServiceNamespace, split.ServiceName, err)
				}

				var targetPort int32
				http2 := false
				for _, port := range service.Spec.Ports {
					if port.Port == split.ServicePort.IntVal || port.Name == split.ServicePort.StrVal {
						targetPort = port.TargetPort.IntVal
						http2 = port.Name == "http2" || port.Name == "h2c"
					}
				}

				publicLbEndpoints := lbEndpointsForKubeEndpoints(endpoints, targetPort)

				connectTimeout := 5 * time.Second
				cluster := envoy.NewCluster(split.ServiceName+path, connectTimeout, publicLbEndpoints, http2, v2.Cluster_STATIC)

				res.clusters = append(res.clusters, cluster)

				weightedCluster := envoy.NewWeightedCluster(split.ServiceName+path, uint32(split.Percent), split.AppendHeaders)

				wrs = append(wrs, weightedCluster)
			}

			if len(wrs) != 0 {
				r := createRouteForRevision(ingress.Name, ingress.Namespace, httpPath, wrs)
				ruleRoute = append(ruleRoute, r)
				res.routes = append(res.routes, r)
			}

		}

		if len(ruleRoute) == 0 {
			// Return nothing if there are not routes to generate.
			return nil, nil
		}

		domains := knative.Domains(rule)
		var virtualHost route.VirtualHost
		if extAuthzEnabled {

			ContextExtensions := map[string]string{
				"client":     "kourier",
				"visibility": string(rule.Visibility),
			}

			ContextExtensions = mergeMapString(ContextExtensions, ingress.GetLabels())
			virtualHost = envoy.NewVirtualHostWithExtAuthz(ingress.Name, ContextExtensions, domains, ruleRoute)
		} else {
			virtualHost = envoy.NewVirtualHost(ingress.GetName(), domains, ruleRoute)
		}

		if knative.RuleIsExternal(rule) {
			res.externalVirtualHosts = append(res.externalVirtualHosts, &virtualHost)
			// External should also be accessible internally
			res.internalVirtualHosts = append(res.internalVirtualHosts, &virtualHost)
		} else {
			res.internalVirtualHosts = append(res.internalVirtualHosts, &virtualHost)
		}
	}

	return res, nil
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

func lbEndpointsForKubeEndpoints(kubeEndpoints *kubev1.Endpoints, targetPort int32) (publicLbEndpoints []*endpoint.LbEndpoint) {
	for _, subset := range kubeEndpoints.Subsets {
		for _, address := range subset.Addresses {
			lbEndpoint := envoy.NewLBEndpoint(address.IP, uint32(targetPort))
			publicLbEndpoints = append(publicLbEndpoints, lbEndpoint)
		}
	}

	return publicLbEndpoints
}

func createRouteForRevision(ingressName string, ingressNamespace string, httpPath v1alpha1.HTTPIngressPath, wrs []*route.WeightedCluster_ClusterWeight) *route.Route {
	routeName := ingressName + "_" + ingressNamespace + "_" + httpPath.Path

	path := "/"
	if httpPath.Path != "" {
		path = httpPath.Path
	}

	var routeTimeout time.Duration
	if httpPath.Timeout != nil {
		routeTimeout = httpPath.Timeout.Duration
	}

	return envoy.NewRoute(
		routeName, matchHeadersFromHTTPPath(httpPath), path, wrs, routeTimeout, httpPath.AppendHeaders,
	)
}

func matchHeadersFromHTTPPath(httpPath v1alpha1.HTTPIngressPath) []*route.HeaderMatcher {
	matchHeaders := make([]*route.HeaderMatcher, 0, len(httpPath.Headers))

	for header, matchType := range httpPath.Headers {
		matchHeader := route.HeaderMatcher{
			Name: header,
		}
		if matchType.Exact != "" {
			matchHeader.HeaderMatchSpecifier = &route.HeaderMatcher_ExactMatch{
				ExactMatch: matchType.Exact,
			}
		}
		matchHeaders = append(matchHeaders, &matchHeader)
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

func mergeMapString(a, b map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}
	return merged
}
