package generator

import (
	"fmt"
	"kourier/pkg/config"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"
	"os"
	"strconv"
	"time"

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
	tracker tracker.Interface) {

	caches.DeleteIngressInfo(ingress.Name, ingress.Namespace, kubeclient)

	// TODO: is this index really needed?
	index := max(
		len(caches.localVirtualHostsForIngress),
		len(caches.externalVirtualHostsForIngress),
	)

	addIngressToCaches(caches, ingress, kubeclient, endpointsLister, localDomainName, index, tracker)

	caches.AddStatusVirtualHost()

	caches.SetListeners(kubeclient)
}

func addIngressToCaches(caches *Caches,
	ingress *v1alpha1.Ingress,
	kubeclient kubeclient.Interface,
	endpointsLister corev1listers.EndpointsLister,
	localDomainName string,
	index int,
	tracker tracker.Interface) {

	var clusterLocalVirtualHosts []*route.VirtualHost
	var externalVirtualHosts []*route.VirtualHost

	caches.AddIngress(ingress)

	log.WithFields(log.Fields{"name": ingress.Name, "namespace": ingress.Namespace}).Info("Knative Ingress found")

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

				caches.AddCluster(&cluster, split.ServiceName, split.ServiceNamespace, path)

				weightedCluster := envoy.NewWeightedCluster(split.ServiceName+path, uint32(split.Percent), headersSplit)

				wrs = append(wrs, &weightedCluster)
			}

			r := createRouteForRevision(ingress.Name, index, &httpPath, wrs)

			ruleRoute = append(ruleRoute, &r)

			caches.AddRoute(&r, ingress.Name, ingress.Namespace)
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
}

func listenersFromVirtualHosts(externalVirtualHosts []*route.VirtualHost,
	clusterLocalVirtualHosts []*route.VirtualHost,
	kubeclient kubeclient.Interface) []*v2.Listener {

	externalManager := envoy.NewHttpConnectionManager(externalVirtualHosts)
	internalManager := envoy.NewHttpConnectionManager(clusterLocalVirtualHosts)

	externalEnvoyListener, err := newExternalEnvoyListener(
		&externalManager,
		kubeclient,
	)
	if err != nil {
		panic(err)
	}

	internalEnvoyListener, err := newInternalEnvoyListener(&internalManager)
	if err != nil {
		panic(err)
	}

	return []*v2.Listener{externalEnvoyListener, internalEnvoyListener}
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

func useHTTPSListener() bool {
	return os.Getenv(envCertsSecretNamespace) != "" &&
		os.Getenv(envCertsSecretName) != ""
}

func sslCreds(kubeClient kubeclient.Interface) (certificateChain string, privateKey string, err error) {
	secret, err := kubeClient.CoreV1().Secrets(os.Getenv(envCertsSecretNamespace)).Get(
		os.Getenv(envCertsSecretName), metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}

	certificateChain = string(secret.Data[certFieldInSecret])
	privateKey = string(secret.Data[keyFieldInSecret])

	return certificateChain, privateKey, nil
}

func newExternalEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager, kubeClient kubeclient.Interface) (*v2.Listener, error) {
	if useHTTPSListener() {
		certificateChain, privateKey, err := sslCreds(kubeClient)

		if err != nil {
			return nil, err
		}

		return envoy.NewHTTPSListener(manager, config.HttpsPortExternal, certificateChain, privateKey)
	}

	return envoy.NewHTTPListener(manager, config.HttpPortExternal)
}

func newInternalEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager) (*v2.Listener, error) {
	return envoy.NewHTTPListener(manager, config.HttpPortInternal)
}

func max(x, y int) int {
	if x >= y {
		return x
	}

	return y
}
