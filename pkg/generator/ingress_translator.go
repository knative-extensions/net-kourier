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
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	pkgconfig "knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/certificates"
	netconfig "knative.dev/networking/pkg/config"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/system"
	"knative.dev/pkg/tracker"
)

type translatedIngress struct {
	name                    types.NamespacedName
	localSNIMatches         []*envoy.SNIMatch
	externalSNIMatches      []*envoy.SNIMatch
	clusters                []*v3.Cluster
	externalVirtualHosts    []*route.VirtualHost
	externalTLSVirtualHosts []*route.VirtualHost
	localVirtualHosts       []*route.VirtualHost
	localTLSVirtualHosts    []*route.VirtualHost
}

type IngressTranslator struct {
	secretGetter      func(ns, name string) (*corev1.Secret, error)
	nsConfigmapGetter func(label string) ([]*corev1.ConfigMap, error)
	endpointsGetter   func(ns, name string) (*corev1.Endpoints, error)
	serviceGetter     func(ns, name string) (*corev1.Service, error)
	tracker           tracker.Interface
}

func NewIngressTranslator(
	secretGetter func(ns, name string) (*corev1.Secret, error),
	nsConfigmapGetter func(label string) ([]*corev1.ConfigMap, error),
	endpointsGetter func(ns, name string) (*corev1.Endpoints, error),
	serviceGetter func(ns, name string) (*corev1.Service, error),
	tracker tracker.Interface) IngressTranslator {
	return IngressTranslator{
		secretGetter:      secretGetter,
		nsConfigmapGetter: nsConfigmapGetter,
		endpointsGetter:   endpointsGetter,
		serviceGetter:     serviceGetter,
		tracker:           tracker,
	}
}

func (translator *IngressTranslator) translateIngress(ctx context.Context, ingress *v1alpha1.Ingress, extAuthzEnabled bool) (*translatedIngress, error) {
	logger := logging.FromContext(ctx)

	localIngressTLS := ingress.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityClusterLocal)
	externalIngressTLS := ingress.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityExternalIP)

	externalSNIMatches := make([]*envoy.SNIMatch, 0, len(externalIngressTLS))
	for _, t := range externalIngressTLS {
		sniMatch, err := translator.translateIngressTLS(t, ingress)
		if err != nil {
			return nil, fmt.Errorf("failed to translate ingressTLS: %w", err)
		}
		externalSNIMatches = append(externalSNIMatches, sniMatch)
	}
	localSNIMatches := make([]*envoy.SNIMatch, 0, len(localIngressTLS))
	for _, t := range localIngressTLS {
		sniMatch, err := translator.translateIngressTLS(t, ingress)
		if err != nil {
			return nil, fmt.Errorf("failed to translate ingressTLS: %w", err)
		}
		localSNIMatches = append(localSNIMatches, sniMatch)
	}

	localHosts := make([]*route.VirtualHost, 0, len(ingress.Spec.Rules))
	localTLSHosts := make([]*route.VirtualHost, 0, len(ingress.Spec.Rules))
	externalHosts := make([]*route.VirtualHost, 0, len(ingress.Spec.Rules))
	externalTLSHosts := make([]*route.VirtualHost, 0, len(ingress.Spec.Rules))
	clusters := make([]*v3.Cluster, 0, len(ingress.Spec.Rules))

	cfg := config.FromContext(ctx)

	var err error
	var trustChain []byte
	if cfg.Network.SystemInternalTLSEnabled() {
		trustChain, err = translator.buildTrustChain(logger)
		if err != nil {
			return nil, err
		}
		if trustChain == nil {
			return nil, fmt.Errorf("failed to build trust-chain, as no valid CA certificate was provided. Please make sure to provide a valid trust-bundle before enabling `system-internal-tls`")
		}
	}

	for i, rule := range ingress.Spec.Rules {
		ruleName := fmt.Sprintf("(%s/%s).Rules[%d]", ingress.Namespace, ingress.Name, i)

		routes := make([]*route.Route, 0, len(rule.HTTP.Paths))
		tlsRoutes := make([]*route.Route, 0, len(rule.HTTP.Paths))
		for _, httpPath := range rule.HTTP.Paths {
			// Default the path to "/" if none is passed.
			path := httpPath.Path
			if path == "" {
				path = "/"
			}

			pathName := fmt.Sprintf("%s.Paths[%s]", ruleName, path)

			wrs := make([]*route.WeightedCluster_ClusterWeight, 0, len(httpPath.Splits))
			for _, split := range httpPath.Splits {
				// The FQN of the service is sufficient here, as clusters towards the
				// same service are supposed to be deduplicated anyway.
				splitName := fmt.Sprintf("%s/%s", split.ServiceNamespace, split.ServiceName)

				if err := trackService(translator.tracker, split.ServiceNamespace, split.ServiceName, ingress); err != nil {
					return nil, err
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
					externalPort = int32(80)
					targetPort   = int32(80)
					http2        = false
				)
				for _, port := range service.Spec.Ports {
					if port.Port == split.ServicePort.IntVal || port.Name == split.ServicePort.StrVal {
						externalPort = port.Port
						targetPort = port.TargetPort.IntVal
					}
					if port.Name == "http2" || port.Name == "h2c" {
						http2 = true
					}
				}

				// Disable HTTP2 if the annotation is specified.
				if strings.EqualFold(pkgconfig.GetDisableHTTP2(ingress.Annotations), "true") {
					http2 = false
				}

				var (
					publicLbEndpoints []*endpoint.LbEndpoint
					typ               v3.Cluster_DiscoveryType
				)
				if service.Spec.Type == corev1.ServiceTypeExternalName {
					// If the service is of type ExternalName, we add a single endpoint.
					typ = v3.Cluster_LOGICAL_DNS
					publicLbEndpoints = []*endpoint.LbEndpoint{
						envoy.NewLBEndpoint(service.Spec.ExternalName, uint32(externalPort)),
					}
				} else {
					// For all other types, fetch the endpoints object.
					endpoints, err := translator.endpointsGetter(split.ServiceNamespace, split.ServiceName)
					if apierrors.IsNotFound(err) {
						logger.Warnf("Endpoints '%s/%s' not yet created", split.ServiceNamespace, split.ServiceName)
						// TODO(markusthoemmes): Find out if we should actually `continue` here.
						return nil, nil
					} else if err != nil {
						return nil, fmt.Errorf("failed to fetch endpoints '%s/%s': %w", split.ServiceNamespace, split.ServiceName, err)
					}

					typ = v3.Cluster_STATIC
					publicLbEndpoints = lbEndpointsForKubeEndpoints(endpoints, targetPort)
				}

				connectTimeout := 5 * time.Second

				var transportSocket *envoycorev3.TransportSocket

				// As Ingress with RewriteHost points to ExternalService(kourier-internal), we don't enable upstream TLS.
				if (cfg.Network.SystemInternalTLSEnabled()) && httpPath.RewriteHost == "" {
					var err error
					transportSocket, err = translator.createUpstreamTransportSocket(http2, split.ServiceNamespace, trustChain)
					if err != nil {
						return nil, err
					}
				}
				cluster := envoy.NewCluster(splitName, connectTimeout, publicLbEndpoints, http2, transportSocket, typ)
				logger.Debugf("adding cluster: %v", cluster)
				clusters = append(clusters, cluster)

				weightedCluster := envoy.NewWeightedCluster(splitName, uint32(split.Percent), split.AppendHeaders)
				wrs = append(wrs, weightedCluster)
			}

			if len(wrs) != 0 {
				// disable ext_authz filter for HTTP01 challenge when the feature is enabled
				if extAuthzEnabled && strings.HasPrefix(path, "/.well-known/acme-challenge/") {
					routes = append(routes, envoy.NewRouteExtAuthzDisabled(
						pathName, matchHeadersFromHTTPPath(httpPath), path, wrs, 0, httpPath.AppendHeaders, httpPath.RewriteHost))
				} else if _, ok := os.LookupEnv("KOURIER_HTTPOPTION_DISABLED"); !ok && ingress.Spec.HTTPOption == v1alpha1.HTTPOptionRedirected && rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
					// Do not create redirect route when KOURIER_HTTPOPTION_DISABLED is set. This option is useful when front end proxy handles the redirection.
					// e.g. Kourier on OpenShift handles HTTPOption by OpenShift Route so KOURIER_HTTPOPTION_DISABLED should be set.
					routes = append(routes, envoy.NewRedirectRoute(
						pathName, matchHeadersFromHTTPPath(httpPath), path))
				} else {
					routes = append(routes, envoy.NewRoute(
						pathName, matchHeadersFromHTTPPath(httpPath), path, wrs, 0, httpPath.AppendHeaders, httpPath.RewriteHost))
				}
				if len(ingress.Spec.TLS) != 0 || useHTTPSListenerWithOneCert() {
					tlsRoutes = append(tlsRoutes, envoy.NewRoute(
						pathName, matchHeadersFromHTTPPath(httpPath), path, wrs, 0, httpPath.AppendHeaders, httpPath.RewriteHost))
				}
			}
		}

		if len(routes) == 0 {
			// Return nothing if there are not routes to generate.
			return nil, nil
		}

		var virtualHost, virtualTLSHost *route.VirtualHost
		// TODO(norbjd): do we want to enable this by default?
		virtualHostOptions := []envoy.VirtualHostOption{
			envoy.WithRetryOnTransientUpstreamFailure(),
		}

		if extAuthzEnabled {
			contextExtensions := kmeta.UnionMaps(map[string]string{
				"client":     "kourier",
				"visibility": string(rule.Visibility),
			}, ingress.GetLabels())
			virtualHostOptions = append(virtualHostOptions, envoy.WithExtAuthz(contextExtensions))
		}

		virtualHost = envoy.NewVirtualHost(ruleName, domainsForRule(rule), routes, virtualHostOptions...)
		if len(tlsRoutes) != 0 {
			virtualTLSHost = envoy.NewVirtualHost(ruleName, domainsForRule(rule), tlsRoutes, virtualHostOptions...)
		}

		localHosts = append(localHosts, virtualHost)
		if rule.Visibility == v1alpha1.IngressVisibilityClusterLocal && virtualTLSHost != nil {
			localTLSHosts = append(localTLSHosts, virtualTLSHost)
		} else if rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			externalHosts = append(externalHosts, virtualHost)
			if virtualTLSHost != nil {
				externalTLSHosts = append(externalTLSHosts, virtualTLSHost)
			}
		}
	}

	return &translatedIngress{
		name: types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      ingress.Name,
		},
		localSNIMatches:         localSNIMatches,
		externalSNIMatches:      externalSNIMatches,
		clusters:                clusters,
		externalVirtualHosts:    externalHosts,
		externalTLSVirtualHosts: externalTLSHosts,
		localVirtualHosts:       localHosts,
		localTLSVirtualHosts:    localTLSHosts,
	}, nil
}

func (translator *IngressTranslator) translateIngressTLS(ingressTLS v1alpha1.IngressTLS, ingress *v1alpha1.Ingress) (*envoy.SNIMatch, error) {
	if err := trackSecret(translator.tracker, ingressTLS.SecretNamespace, ingressTLS.SecretName, ingress); err != nil {
		return nil, err
	}

	secret, err := translator.secretGetter(ingressTLS.SecretNamespace, ingressTLS.SecretName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secret: %w", err)
	}

	// Validate certificate here as these are defined by users.
	// We should not send Gateway without validation.
	_, err = tls.X509KeyPair(
		secret.Data[certificates.CertName],
		secret.Data[certificates.PrivateKeyName],
	)
	if err != nil {
		return nil, fmt.Errorf("invalid secret is specified: %w", err)
	}

	secretRef := types.NamespacedName{
		Namespace: ingressTLS.SecretNamespace,
		Name:      ingressTLS.SecretName,
	}
	sniMatch := &envoy.SNIMatch{
		Hosts:            ingressTLS.Hosts,
		CertSource:       secretRef,
		CertificateChain: secret.Data[certificates.CertName],
		PrivateKey:       secret.Data[certificates.PrivateKeyName],
	}
	return sniMatch, nil
}

func (translator *IngressTranslator) createUpstreamTransportSocket(http2 bool, namespace string, trustChain []byte) (*envoycorev3.TransportSocket, error) {
	var alpnProtocols string
	if http2 {
		alpnProtocols = "h2"
	}
	tlsAny, err := anypb.New(createUpstreamTLSContext(trustChain, namespace, alpnProtocols))
	if err != nil {
		return nil, err
	}
	return &envoycorev3.TransportSocket{
		Name: wellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: tlsAny,
		},
	}, nil
}

func createUpstreamTLSContext(trustChain []byte, namespace string, alpnProtocols ...string) *tlsv3.UpstreamTlsContext {
	return &tlsv3.UpstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			AlpnProtocols: alpnProtocols,
			TlsParams: &tlsv3.TlsParameters{
				TlsMinimumProtocolVersion: tlsv3.TlsParameters_TLSv1_3,
				TlsMaximumProtocolVersion: tlsv3.TlsParameters_TLSv1_3,
			},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineBytes{
							InlineBytes: trustChain,
						},
					},
					MatchTypedSubjectAltNames: []*tlsv3.SubjectAltNameMatcher{{
						SanType: tlsv3.SubjectAltNameMatcher_DNS,
						Matcher: &envoymatcherv3.StringMatcher{
							MatchPattern: &envoymatcherv3.StringMatcher_Exact{
								// SAN used by Activator
								Exact: certificates.DataPlaneRoutingSAN,
							},
						},
					}, {
						SanType: tlsv3.SubjectAltNameMatcher_DNS,
						Matcher: &envoymatcherv3.StringMatcher{
							MatchPattern: &envoymatcherv3.StringMatcher_Exact{
								// SAN used by Queue-Proxy in target namespace
								Exact: certificates.DataPlaneUserSAN(namespace),
							},
						},
					}},
				},
			},
		},
	}
}

// CA can optionally be in `ca.crt` in the `routing-serving-certs` secret
// and/or configured using a trust-bundle via ConfigMap that has the defined label `knative-ca-trust-bundle`.
// Our upstream TLS context needs to trust them all.
func (translator *IngressTranslator) buildTrustChain(logger *zap.SugaredLogger) ([]byte, error) {
	var trustChain []byte

	routingCA, err := translator.secretGetter(system.Namespace(), netconfig.ServingRoutingCertName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Secret %s/%s: %w", system.Namespace(), netconfig.ServingRoutingCertName, err)
	}
	routingCABytes := routingCA.Data[certificates.CaCertName]
	if len(routingCABytes) > 0 {
		if err = checkCertBundle(routingCABytes); err != nil {
			logger.Warnf("CA from Secret %s/%s[%s] is invalid and will be ignored: %v",
				system.Namespace(), netconfig.ServingRoutingCertName, certificates.CaCertName, err)
		} else {
			logger.Debugf("Adding CA from Secret %s/%s[%s] to trust chain", system.Namespace(), netconfig.ServingRoutingCertName, certificates.CaCertName)
			trustChain = routingCABytes
		}
	}

	cms, err := translator.nsConfigmapGetter(networking.TrustBundleLabelKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Configmaps with label: %s in namespace: %s: %w", networking.TrustBundleLabelKey, system.Namespace(), err)
	}

	newline := []byte("\n")
	for _, cm := range cms {
		for _, bundle := range cm.Data {
			if err = checkCertBundle([]byte(bundle)); err != nil {
				logger.Warnf("CA bundle from Configmap %s/%s is invalid and will be ignored: %v",
					system.Namespace(), cm.Name, err)
			} else {
				logger.Debugf("Adding CA bundle from Configmap %s/%s to trust chain", system.Namespace(), cm.Name)
				if len(trustChain) > 0 {
					// Make sure we always have at least one newline between bundles, multiple ones are ok
					trustChain = append(trustChain, newline...)
				}
				trustChain = append(trustChain, []byte(bundle)...)
			}
		}
	}

	return trustChain, nil
}

func trackSecret(t tracker.Interface, ns, name string, ingress *v1alpha1.Ingress) error {
	return t.TrackReference(tracker.Reference{
		Kind:       "Secret",
		APIVersion: "v1",
		Namespace:  ns,
		Name:       name,
	}, ingress)
}

func trackService(t tracker.Interface, svcNs, svcName string, ingress *v1alpha1.Ingress) error {
	if err := t.TrackReference(tracker.Reference{
		Kind:       "Service",
		APIVersion: "v1",
		Namespace:  svcNs,
		Name:       svcName,
	}, ingress); err != nil {
		return fmt.Errorf("could not track service reference: %w", err)
	}

	if err := t.TrackReference(tracker.Reference{
		Kind:       "Endpoints",
		APIVersion: "v1",
		Namespace:  svcNs,
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

			matchHeader.HeaderMatchSpecifier = &route.HeaderMatcher_StringMatch{
				StringMatch: &envoymatcherv3.StringMatcher{
					MatchPattern: &envoymatcherv3.StringMatcher_Exact{
						Exact: matchType.Exact,
					},
				},
			}
		}
		matchHeaders = append(matchHeaders, matchHeader)
	}
	return matchHeaders
}

// domainsForRule returns all domains for the given rule.
//
// For example, external domains returns domains with the following formats:
//   - sub-route_host.namespace.example.com
//   - sub-route_host.namespace.example.com:*
//
// Somehow envoy doesn't match properly gRPC authorities with ports.
// The fix is to include ":*" in the domains.
// This applies both for local and external domains.
// More info https://github.com/envoyproxy/envoy/issues/886
func domainsForRule(rule v1alpha1.IngressRule) []string {
	domains := make([]string, 0, 2*len(rule.Hosts))
	for _, host := range rule.Hosts {
		domains = append(domains, host, host+":*")
	}
	return domains
}

func checkCertBundle(certs []byte) error {
	for block, rest := pem.Decode(certs); block != nil || len(rest) > 0; block, rest = pem.Decode(rest) {
		if block != nil {
			switch block.Type {
			case "CERTIFICATE":
				_, err := x509.ParseCertificate(block.Bytes)
				if err != nil {
					return fmt.Errorf("failed to parse certificate. Invalid certificate found: %s, %v", block.Bytes, err.Error())
				}

			default:
				return fmt.Errorf("failed to parse bundle. Bundle contains something other than a certificate. Type: %s, block: %s", block.Type, block.Bytes)
			}
		} else {
			// if the last certificate is parsed, and we still have rest, there are additional unwanted things in the CM
			return fmt.Errorf("failed to parse bundle. Bundle contains something other than a certificate: %s", rest)
		}
	}
	return nil
}
