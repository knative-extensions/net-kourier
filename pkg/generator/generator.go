package generator

import (
	"fmt"
	"kourier/pkg/config"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"
	"os"
	"strconv"
	"time"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/duration"

	httpconnmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"

	"knative.dev/pkg/tracker"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	log "github.com/sirupsen/logrus"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/pkg/reconciler/ingress/resources"
)

const (
	envCertsSecretNamespace = "CERTS_SECRET_NAMESPACE"
	envCertsSecretName      = "CERTS_SECRET_NAME"
	certFieldInSecret       = "tls.crt"
	keyFieldInSecret        = "tls.key"
)

// For now, when updating the info for an ingress we delete it, and then
// regenerate it. We can optimize this later.
func UpdateInfoForIngress(caches *Caches,
	ingress *v1alpha1.Ingress,
	kubeclient kubeclient.Interface,
	endpointsLister corev1listers.EndpointsLister,
	localDomainName string,
	tracker tracker.Interface) error {

	caches.DeleteIngressInfo(ingress.Name, ingress.Namespace, kubeclient)

	// TODO: is this index really needed?
	index := max(
		len(caches.localVirtualHostsForIngress),
		len(caches.externalVirtualHostsForIngress),
	)

	err := addIngressToCaches(caches, ingress, kubeclient, endpointsLister, localDomainName, index, tracker)

	if err != nil {
		return err
	}

	caches.AddStatusVirtualHost()

	err = caches.SetListeners(kubeclient)
	if err != nil {
		return err
	}

	return nil
}

func addIngressToCaches(caches *Caches,
	ingress *v1alpha1.Ingress,
	kubeclient kubeclient.Interface,
	endpointsLister corev1listers.EndpointsLister,
	localDomainName string,
	index int,
	tracker tracker.Interface) error {

	var clusterLocalVirtualHosts []*route.VirtualHost
	var externalVirtualHosts []*route.VirtualHost

	caches.AddIngress(ingress)

	log.WithFields(log.Fields{"name": ingress.Name, "namespace": ingress.Namespace}).Info("Knative Ingress found")

	for _, ingressTLS := range ingress.GetSpec().TLS {
		sniMatch, err := sniMatchFromIngressTLS(ingressTLS, kubeclient)

		if err != nil {
			log.Errorf("%s", err)

			// We need to propagate this error to the reconciler so the current
			// event can be retried. This error might be caused because the
			// secrets referenced in the TLS section of the spec do not exist
			// yet. That's expected when auto TLS is configured.
			// See the "TestPerKsvcCert_localCA" test in Knative Serving. It's a
			// test that fails if this error is not propagated:
			// https://github.com/knative/serving/blob/571e4db2392839082c559870ea8d4b72ef61e59d/test/e2e/autotls/auto_tls_test.go#L68
			return err
		} else {
			caches.AddSNIMatch(sniMatch, ingress.Name, ingress.Namespace)
		}
	}

	for _, rule := range ingress.GetSpec().Rules {

		var ruleRoute []*route.Route

		for _, httpPath := range rule.HTTP.Paths {

			path := "/"
			if httpPath.Path != "" {
				path = httpPath.Path
			}

			var wrs []*route.WeightedCluster_ClusterWeight

			for _, split := range httpPath.Splits {

				headersSplit := split.AppendHeaders

				endpoints, err := endpointsLister.Endpoints(split.ServiceNamespace).Get(split.ServiceName)
				if err != nil {
					log.Errorf("%s", err)
					break
				}

				ref := kubev1.ObjectReference{
					Kind:       "Endpoints",
					APIVersion: "v1",
					Namespace:  ingress.Namespace,
					Name:       split.ServiceName,
				}

				if tracker != nil {
					err = tracker.Track(ref, ingress)
					if err != nil {
						log.Errorf("%s", err)
						break
					}
				}

				service, err := kubeclient.CoreV1().Services(split.ServiceNamespace).Get(split.ServiceName, metav1.GetOptions{})
				if err != nil {
					log.Errorf("%s", err)
					break
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
				cluster := envoy.NewCluster(split.ServiceName+path, connectTimeout, publicLbEndpoints, http2)

				caches.AddCluster(&cluster, ingress.Name, ingress.Namespace)

				weightedCluster := envoy.NewWeightedCluster(split.ServiceName+path, uint32(split.Percent), headersSplit)

				wrs = append(wrs, &weightedCluster)
			}

			if len(wrs) != 0 {
				r := createRouteForRevision(ingress.Name, index, &httpPath, wrs)

				ruleRoute = append(ruleRoute, &r)

				caches.AddRoute(&r, ingress.Name, ingress.Namespace)
			}

		}

		if len(ruleRoute) == 0 {
			log.Info("No rules for this ingress, returning.")
			return nil
		}

		externalDomains := knative.ExternalDomains(&rule, localDomainName)
		virtualHost := envoy.NewVirtualHost(ingress.Name, externalDomains, ruleRoute)

		// External should also be accessible internally
		internalDomains := append(knative.InternalDomains(&rule, localDomainName), externalDomains...)
		internalVirtualHost := envoy.NewVirtualHost(ingress.Name, internalDomains, ruleRoute)

		if knative.RuleIsExternal(&rule, ingress.GetSpec().Visibility) {
			externalVirtualHosts = append(externalVirtualHosts, &virtualHost)
		}

		clusterLocalVirtualHosts = append(clusterLocalVirtualHosts, &internalVirtualHost)
	}

	for _, vHost := range externalVirtualHosts {
		caches.AddExternalVirtualHostForIngress(vHost, ingress.Name, ingress.Namespace)
	}

	for _, vHost := range clusterLocalVirtualHosts {
		caches.AddInternalVirtualHostForIngress(vHost, ingress.Name, ingress.Namespace)
	}

	return nil
}

func listenersFromVirtualHosts(externalVirtualHosts []*route.VirtualHost,
	clusterLocalVirtualHosts []*route.VirtualHost,
	sniMatches []*envoy.SNIMatch,
	kubeclient kubeclient.Interface, caches *Caches) ([]*v2.Listener, error) {

	var listeners []*v2.Listener

	externalManager := envoy.NewHttpConnectionManager(externalVirtualHosts)
	internalManager := envoy.NewHttpConnectionManager(clusterLocalVirtualHosts)

	internalRouteConfig := internalManager.GetRouteConfig()
	externalRouteConfig := externalManager.GetRouteConfig()

	// We need to keep these for the case of HTTPS with SNI routing
	originalExternalManager := externalManager
	originalExternalVHosts := externalRouteConfig.VirtualHosts

	// Set proper names so those can be referred later.
	internalRouteConfig.Name = "internal_services"
	externalRouteConfig.Name = "external_services"

	// Now we save the RouteConfigs with the proper name and all the virtualhosts etc.. into the cache.
	caches.routeConfig = []v2.RouteConfiguration{}
	caches.routeConfig = append(caches.routeConfig, *externalRouteConfig)
	caches.routeConfig = append(caches.routeConfig, *internalRouteConfig)

	// Now let's forget about the cache, and override the internal manager to point to the RDS and look for the proper
	// names.
	internalManager.RouteSpecifier = &httpconnmanagerv2.HttpConnectionManager_Rds{
		Rds: &httpconnmanagerv2.Rds{
			ConfigSource: &envoy_api_v2_core.ConfigSource{
				ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_Ads{
					Ads: &envoy_api_v2_core.AggregatedConfigSource{},
				},
				InitialFetchTimeout: &duration.Duration{
					Seconds: 10,
					Nanos:   0,
				},
			},
			RouteConfigName: "internal_services",
		},
	}
	// Set the discovery to ADS

	externalManager.RouteSpecifier = &httpconnmanagerv2.HttpConnectionManager_Rds{
		Rds: &httpconnmanagerv2.Rds{
			ConfigSource: &envoy_api_v2_core.ConfigSource{
				ConfigSourceSpecifier: &envoy_api_v2_core.ConfigSource_Ads{
					Ads: &envoy_api_v2_core.AggregatedConfigSource{},
				},
				InitialFetchTimeout: &duration.Duration{
					Seconds: 10,
					Nanos:   0,
				},
			},
			RouteConfigName: "external_services",
		},
	}

	// CleanUp virtual hosts.
	externalRouteConfig.VirtualHosts = []*route.VirtualHost{}
	internalRouteConfig.VirtualHosts = []*route.VirtualHost{}

	externalHTTPEnvoyListener, err := newExternalHTTPEnvoyListener(&externalManager)
	if err != nil {
		return nil, err
	}
	listeners = append(listeners, externalHTTPEnvoyListener)

	internalEnvoyListener, err := newInternalEnvoyListener(&internalManager)
	if err != nil {
		return nil, err
	}
	listeners = append(listeners, internalEnvoyListener)

	// Configure TLS Listener. If there's at least one ingress that contains the
	// TLS field, that takes precedence. If there is not, TLS will be configured
	// using a single cert for all the services if the creds are given via ENV.
	if len(sniMatches) > 0 {
		// TODO: Can we make this work with "HttpConnectionManager_Rds"?
		externalRouteConfig.VirtualHosts = originalExternalVHosts
		externalHTTPSEnvoyListener, err := newExternalHTTPSEnvoyListener(
			&originalExternalManager, sniMatches,
		)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, externalHTTPSEnvoyListener)
	} else if useHTTPSListenerWithOneCert() {
		externalHTTPSEnvoyListener, err := newExternalEnvoyListenerWithOneCert(
			&externalManager, kubeclient,
		)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, externalHTTPSEnvoyListener)
	}

	return listeners, nil
}

func internalKourierVirtualHost(ikrs []*route.Route) route.VirtualHost {
	return envoy.NewVirtualHost(
		config.InternalKourierDomain,
		[]string{config.InternalKourierDomain},
		ikrs,
	)
}

func internalKourierRoutes(ingresses []*v1alpha1.Ingress) []*route.Route {
	var hashes []string
	var routes []*route.Route
	for _, ingress := range ingresses {
		hash, err := resources.ComputeIngressHash(ingress)
		if err != nil {
			log.Errorf("Failed to hash ingress %s: %s", ingress.Name, err)
			break
		}
		hashes = append(hashes, fmt.Sprintf("%x", hash))
	}

	for _, hash := range hashes {
		name := fmt.Sprintf("%s_%s", config.InternalKourierDomain, hash)
		path := fmt.Sprintf("%s/%s", config.InternalKourierPath, hash)
		r := envoy.NewRouteStatusOK(name, path)
		routes = append(routes, &r)
	}

	staticRoute := envoy.NewRouteStatusOK(
		config.InternalKourierDomain,
		config.InternalKourierPath,
	)
	routes = append(routes, &staticRoute)

	return routes
}

func lbEndpointsForKubeEndpoints(kubeEndpoints *kubev1.Endpoints, targetPort int32) (publicLbEndpoints []*endpoint.LbEndpoint) {
	for _, subset := range kubeEndpoints.Subsets {
		for _, address := range subset.Addresses {
			lbEndpoint := envoy.NewLBEndpoint(address.IP, uint32(targetPort))
			publicLbEndpoints = append(publicLbEndpoints, &lbEndpoint)
		}
	}

	return publicLbEndpoints
}

func createRouteForRevision(routeName string, i int, httpPath *v1alpha1.HTTPIngressPath, wrs []*route.WeightedCluster_ClusterWeight) route.Route {
	name := routeName + "_" + strconv.Itoa(i)

	path := "/"
	if httpPath.Path != "" {
		path = httpPath.Path
	}

	var routeTimeout time.Duration
	if httpPath.Timeout != nil {
		routeTimeout = httpPath.Timeout.Duration
	}

	attempts := 0
	var perTryTimeout time.Duration
	if httpPath.Retries != nil {
		attempts = httpPath.Retries.Attempts

		if httpPath.Retries.PerTryTimeout != nil {
			perTryTimeout = httpPath.Retries.PerTryTimeout.Duration
		}
	}

	return envoy.NewRoute(
		name, path, wrs, routeTimeout, uint32(attempts), perTryTimeout, httpPath.AppendHeaders,
	)
}

// Returns true if we need to modify the HTTPS listener with just one cert
// instead of one per ingress
func useHTTPSListenerWithOneCert() bool {
	return os.Getenv(envCertsSecretNamespace) != "" &&
		os.Getenv(envCertsSecretName) != ""
}

func sslCreds(kubeClient kubeclient.Interface, secretNamespace string, secretName string) (certificateChain string, privateKey string, err error) {
	secret, err := kubeClient.CoreV1().Secrets(secretNamespace).Get(secretName, metav1.GetOptions{})

	if err != nil {
		return "", "", err
	}

	certificateChain = string(secret.Data[certFieldInSecret])
	privateKey = string(secret.Data[keyFieldInSecret])

	return certificateChain, privateKey, nil
}

func sniMatchFromIngressTLS(ingressTLS v1alpha1.IngressTLS, kubeClient kubeclient.Interface) (*envoy.SNIMatch, error) {
	certChain, privateKey, err := sslCreds(
		kubeClient, ingressTLS.SecretNamespace, ingressTLS.SecretName,
	)

	if err != nil {
		return nil, err
	}

	sniMatch := envoy.NewSNIMatch(ingressTLS.Hosts, certChain, privateKey)
	return &sniMatch, nil
}

func newExternalEnvoyListenerWithOneCert(manager *httpconnmanagerv2.HttpConnectionManager, kubeClient kubeclient.Interface) (*v2.Listener, error) {
	certificateChain, privateKey, err := sslCreds(
		kubeClient, os.Getenv(envCertsSecretNamespace), os.Getenv(envCertsSecretName),
	)

	if err != nil {
		return nil, err
	}

	return envoy.NewHTTPSListener(manager, config.HttpsPortExternal, certificateChain, privateKey)
}

func newExternalHTTPEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager) (*v2.Listener, error) {
	return envoy.NewHTTPListener(manager, config.HttpPortExternal)
}

func newInternalEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager) (*v2.Listener, error) {
	return envoy.NewHTTPListener(manager, config.HttpPortInternal)
}

func newExternalHTTPSEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager, sniMatches []*envoy.SNIMatch) (*v2.Listener, error) {
	return envoy.NewHTTPSListenerWithSNI(manager, config.HttpsPortExternal, sniMatches)
}

func max(x, y int) int {
	if x >= y {
		return x
	}

	return y
}
