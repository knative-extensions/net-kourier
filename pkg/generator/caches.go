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
	"errors"
	"os"
	"strconv"
	"sync"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	httpconnmanagerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	rconfig "knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/pkg/system"
)

const (
	envCertsSecretNamespace    = "CERTS_SECRET_NAMESPACE"
	envCertsSecretName         = "CERTS_SECRET_NAME"
	certFieldInSecret          = "tls.crt"
	keyFieldInSecret           = "tls.key"
	externalRouteConfigName    = "external_services"
	externalTLSRouteConfigName = "external_tls_services"
	internalRouteConfigName    = "internal_services"
	isolationRouteConfigName   = "isolation_services"
	internalTLSRouteConfigName = "internal_tls_services"
)

// ErrDomainConflict is an error produces when two ingresses have conflicting domains.
var ErrDomainConflict = errors.New("ingress has a conflicting domain with another ingress")

type Caches struct {
	mu                  sync.Mutex
	translatedIngresses map[types.NamespacedName]*translatedIngress
	clusters            *ClustersCache
	domainsInUse        sets.String
	statusVirtualHost   *route.VirtualHost

	kubeClient kubeclient.Interface
}

type portVHost struct {
	port  string
	vhost []*route.VirtualHost
}

func NewCaches(ctx context.Context, kubernetesClient kubeclient.Interface, extAuthz bool) (*Caches, error) {
	c := &Caches{
		translatedIngresses: make(map[types.NamespacedName]*translatedIngress),
		clusters:            newClustersCache(),
		domainsInUse:        sets.NewString(),
		statusVirtualHost:   statusVHost(),
		kubeClient:          kubernetesClient,
	}

	if extAuthz {
		c.clusters.set(config.ExternalAuthz.Cluster, "__extAuthZCluster", "_internal")
	}
	return c, nil
}

func (caches *Caches) UpdateIngress(ctx context.Context, ingressTranslation *translatedIngress) error {
	// we hold a lock for Updating the ingress, to avoid another worker to generate an snapshot just when we have
	// deleted the ingress before adding it.
	caches.mu.Lock()
	defer caches.mu.Unlock()

	caches.deleteTranslatedIngress(ingressTranslation.name.Name, ingressTranslation.name.Namespace)
	return caches.addTranslatedIngress(ingressTranslation)
}

func (caches *Caches) validateIngress(translatedIngress *translatedIngress) error {
	for _, vhost := range translatedIngress.internalVirtualHosts {
		if caches.domainsInUse.HasAny(vhost.Domains...) {
			return ErrDomainConflict
		}
	}

	return nil
}

func (caches *Caches) addTranslatedIngress(translatedIngress *translatedIngress) error {
	if err := caches.validateIngress(translatedIngress); err != nil {
		return err
	}

	for _, vhost := range translatedIngress.internalVirtualHosts {
		caches.domainsInUse.Insert(vhost.Domains...)
	}

	caches.translatedIngresses[translatedIngress.name] = translatedIngress

	for _, cluster := range translatedIngress.clusters {
		caches.clusters.set(cluster, translatedIngress.name.Name, translatedIngress.name.Namespace)
	}

	return nil
}

// SetOnEvicted allows to set a function that will be executed when any key on the cache expires.
func (caches *Caches) SetOnEvicted(f func(types.NamespacedName, interface{})) {
	caches.clusters.clusters.OnEvicted(func(key string, val interface{}) {
		_, name, namespace := explodeKey(key)
		f(types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, val)
	})
}

func (caches *Caches) ToEnvoySnapshot(ctx context.Context) (*cache.Snapshot, error) {
	caches.mu.Lock()
	defer caches.mu.Unlock()

	localVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses)+1)
	externalVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses))
	externalTLSVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses))
	snis := sniMatches{}

	localVHostsPerListener := make(map[string]portVHost)

	for _, translatedIngress := range caches.translatedIngresses {
		if translatedIngress.listenerPort != "" {
			localVHostsPerListener[translatedIngress.listenerPort] = portVHost{
				port:  translatedIngress.listenerPort,
				vhost: append(localVHostsPerListener[translatedIngress.listenerPort].vhost, translatedIngress.internalVirtualHosts...),
			}
		} else {
			localVHosts = append(localVHosts, translatedIngress.internalVirtualHosts...)
		}

		externalVHosts = append(externalVHosts, translatedIngress.externalVirtualHosts...)
		externalTLSVHosts = append(externalTLSVHosts, translatedIngress.externalTLSVirtualHosts...)

		for _, match := range translatedIngress.sniMatches {
			snis.consume(match)
		}
	}

	// Append the statusHost too.
	localVHosts = append(localVHosts, caches.statusVirtualHost)

	listeners, routes, err := generateListenersAndRouteConfigs(
		ctx,
		externalVHosts,
		externalTLSVHosts,
		localVHosts,
		localVHostsPerListener,
		snis.list(),
		caches.kubeClient,
	)
	if err != nil {
		return nil, err
	}

	return cache.NewSnapshot(
		uuid.NewString(),
		map[resource.Type][]cachetypes.Resource{
			resource.ClusterType:  caches.clusters.list(),
			resource.RouteType:    routes,
			resource.ListenerType: listeners,
		},
	)
}

// DeleteIngressInfo removes an ingress from the caches.
//
// Notice that the clusters are not deleted. That's handled with the expiration
// time set in the "ClustersCache" struct.
func (caches *Caches) DeleteIngressInfo(ctx context.Context, ingressName string, ingressNamespace string) error {
	caches.mu.Lock()
	defer caches.mu.Unlock()

	caches.deleteTranslatedIngress(ingressName, ingressNamespace)
	return nil
}

func (caches *Caches) deleteTranslatedIngress(ingressName, ingressNamespace string) {
	key := types.NamespacedName{
		Namespace: ingressNamespace,
		Name:      ingressName,
	}

	// Set to expire all the clusters belonging to that Ingress.
	if translated := caches.translatedIngresses[key]; translated != nil {
		for _, cluster := range translated.clusters {
			caches.clusters.setExpiration(cluster.Name, ingressName, ingressNamespace)
		}

		for _, vhost := range translated.internalVirtualHosts {
			caches.domainsInUse.Delete(vhost.Domains...)
		}

		delete(caches.translatedIngresses, key)
	}
}

func generateListenersAndRouteConfigs(
	ctx context.Context,
	externalVirtualHosts []*route.VirtualHost,
	externalTLSVirtualHosts []*route.VirtualHost,
	clusterLocalVirtualHosts []*route.VirtualHost,
	clusterLocalVirtualHostsPerListener map[string]portVHost,
	sniMatches []*envoy.SNIMatch,
	kubeclient kubeclient.Interface) ([]cachetypes.Resource, []cachetypes.Resource, error) {

	// This has to be "OrDefaults" because this path is called before the informers are
	// running when booting the controller up and prefilling the config before making it
	// ready.
	cfg := rconfig.FromContextOrDefaults(ctx)

	// First, we save the RouteConfigs with the proper name and all the virtualhosts etc. into the cache.
	externalRouteConfig := envoy.NewRouteConfig(externalRouteConfigName, externalVirtualHosts)
	externalTLSRouteConfig := envoy.NewRouteConfig(externalTLSRouteConfigName, externalTLSVirtualHosts)
	internalRouteConfig := envoy.NewRouteConfig(internalRouteConfigName, clusterLocalVirtualHosts)

	internalListenersRouteConfig := make(map[string]*route.RouteConfiguration, len(clusterLocalVirtualHostsPerListener))
	for listenerPort, portVhosts := range clusterLocalVirtualHostsPerListener {
		routeName := isolationRouteConfigName + "_" + listenerPort
		internalListenersRouteConfig[listenerPort] = envoy.NewRouteConfig(routeName, portVhosts.vhost)
	}

	// Now we setup connection managers, that reference the routeconfigs via RDS.
	externalManager := envoy.NewHTTPConnectionManager(externalRouteConfig.Name, cfg.Kourier)
	externalTLSManager := envoy.NewHTTPConnectionManager(externalTLSRouteConfig.Name, cfg.Kourier)
	internalManager := envoy.NewHTTPConnectionManager(internalRouteConfig.Name, cfg.Kourier)

	internalListenerManagers := make(map[string]*httpconnmanagerv3.HttpConnectionManager, len(internalListenersRouteConfig))
	for listenerPort, internalListenerRouteConfig := range internalListenersRouteConfig {
		internalListenerManagers[listenerPort] = envoy.NewHTTPConnectionManager(internalListenerRouteConfig.Name, cfg.Kourier)
	}

	externalHTTPEnvoyListener, err := envoy.NewHTTPListener(externalManager, config.HTTPPortExternal, cfg.Kourier.EnableProxyProtocol)
	if err != nil {
		return nil, nil, err
	}
	internalEnvoyListener, err := envoy.NewHTTPListener(internalManager, config.HTTPPortInternal, false)
	if err != nil {
		return nil, nil, err
	}

	listeners := []cachetypes.Resource{externalHTTPEnvoyListener, internalEnvoyListener}
	routes := []cachetypes.Resource{externalRouteConfig, internalRouteConfig}

	for listenerPort, portVhosts := range clusterLocalVirtualHostsPerListener {
		port, err := strconv.ParseInt(portVhosts.port, 10, 32)
		if err != nil {
			return nil, nil, err
		}

		envoyListener, err := envoy.NewHTTPListener(internalListenerManagers[listenerPort], uint32(port), false)
		if err != nil {
			return nil, nil, err
		}
		listeners = append(listeners, envoyListener)
		routes = append(routes, internalListenersRouteConfig[listenerPort])
	}

	// create probe listeners
	probHTTPListener, err := envoy.NewHTTPListener(externalManager, config.HTTPPortProb, false)
	if err != nil {
		return nil, nil, err
	}
	listeners = append(listeners, probHTTPListener)

	// Add internal listeners and routes when internal cert secret is specified.
	if cfg.Kourier.ClusterCertSecret != "" {
		internalTLSRouteConfig := envoy.NewRouteConfig(internalTLSRouteConfigName, clusterLocalVirtualHosts)
		internalTLSManager := envoy.NewHTTPConnectionManager(internalTLSRouteConfig.Name, cfg.Kourier)

		internalHTTPSEnvoyListener, err := newInternalEnvoyListenerWithOneCert(
			ctx, internalTLSManager, kubeclient,
			cfg.Kourier,
		)

		if err != nil {
			return nil, nil, err
		}

		listeners = append(listeners, internalHTTPSEnvoyListener)
		routes = append(routes, internalTLSRouteConfig)
	}

	// Configure TLS Listener. If there's at least one ingress that contains the
	// TLS field, that takes precedence. If there is not, TLS will be configured
	// using a single cert for all the services if the creds are given via ENV.
	if len(sniMatches) > 0 {
		externalHTTPSEnvoyListener, err := envoy.NewHTTPSListenerWithSNI(
			externalTLSManager, config.HTTPSPortExternal,
			sniMatches, cfg.Kourier.EnableProxyProtocol,
		)
		if err != nil {
			return nil, nil, err
		}

		// create https prob listener with SNI
		probHTTPSListener, err := envoy.NewHTTPSListenerWithSNI(
			externalManager, config.HTTPSPortProb,
			sniMatches, false,
		)
		if err != nil {
			return nil, nil, err
		}

		// if a certificate is configured, add a new filter chain to TLS listener
		if useHTTPSListenerWithOneCert() {
			externalHTTPSEnvoyListenerWithOneCertFilterChain, err := newExternalEnvoyListenerWithOneCertFilterChain(
				ctx, externalTLSManager, kubeclient, cfg.Kourier,
			)
			if err != nil {
				return nil, nil, err
			}

			externalHTTPSEnvoyListener.FilterChains = append(externalHTTPSEnvoyListener.FilterChains,
				externalHTTPSEnvoyListenerWithOneCertFilterChain)
			probHTTPSListener.FilterChains = append(probHTTPSListener.FilterChains,
				externalHTTPSEnvoyListenerWithOneCertFilterChain)
		}

		listeners = append(listeners, externalHTTPSEnvoyListener, probHTTPSListener)
		routes = append(routes, externalTLSRouteConfig)
	} else if useHTTPSListenerWithOneCert() {
		externalHTTPSEnvoyListener, err := newExternalEnvoyListenerWithOneCert(
			ctx, externalTLSManager, kubeclient,
			cfg.Kourier,
		)
		if err != nil {
			return nil, nil, err
		}

		// create https prob listener
		probHTTPSListener, err := envoy.NewHTTPSListener(config.HTTPSPortProb, externalHTTPSEnvoyListener.FilterChains, false)
		if err != nil {
			return nil, nil, err
		}

		listeners = append(listeners, externalHTTPSEnvoyListener, probHTTPSListener)
		routes = append(routes, externalTLSRouteConfig)
	}

	return listeners, routes, nil
}

// Returns true if we need to modify the HTTPS listener with just one cert
// instead of one per ingress
func useHTTPSListenerWithOneCert() bool {
	return os.Getenv(envCertsSecretNamespace) != "" &&
		os.Getenv(envCertsSecretName) != ""
}

func sslCreds(ctx context.Context, kubeClient kubeclient.Interface, secretNamespace string, secretName string) (certificateChain []byte, privateKey []byte, err error) {
	secret, err := kubeClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	return secret.Data[certFieldInSecret], secret.Data[keyFieldInSecret], nil
}

func newExternalEnvoyListenerWithOneCertFilterChain(ctx context.Context, manager *httpconnmanagerv3.HttpConnectionManager, kubeClient kubeclient.Interface, cfg *config.Kourier) (*v3.FilterChain, error) {
	certificateChain, privateKey, err := sslCreds(
		ctx, kubeClient, os.Getenv(envCertsSecretNamespace), os.Getenv(envCertsSecretName),
	)
	if err != nil {
		return nil, err
	}

	return envoy.CreateFilterChainFromCertificateAndPrivateKey(manager, &envoy.Certificate{
		Certificate:        certificateChain,
		PrivateKey:         privateKey,
		PrivateKeyProvider: privateKeyProvider(cfg.EnableCryptoMB),
	})
}

func newExternalEnvoyListenerWithOneCert(ctx context.Context, manager *httpconnmanagerv3.HttpConnectionManager, kubeClient kubeclient.Interface, cfg *config.Kourier) (*v3.Listener, error) {
	filterChain, err := newExternalEnvoyListenerWithOneCertFilterChain(ctx, manager, kubeClient, cfg)
	if err != nil {
		return nil, err
	}

	return envoy.NewHTTPSListener(config.HTTPSPortExternal, []*v3.FilterChain{filterChain}, cfg.EnableProxyProtocol)
}

func newInternalEnvoyListenerWithOneCert(ctx context.Context, manager *httpconnmanagerv3.HttpConnectionManager, kubeClient kubeclient.Interface, cfg *config.Kourier) (*v3.Listener, error) {
	certificateChain, privateKey, err := sslCreds(ctx, kubeClient, system.Namespace(), cfg.ClusterCertSecret)
	if err != nil {
		return nil, err
	}
	filterChain, err := envoy.CreateFilterChainFromCertificateAndPrivateKey(manager, &envoy.Certificate{
		Certificate:        certificateChain,
		PrivateKey:         privateKey,
		PrivateKeyProvider: privateKeyProvider(cfg.EnableCryptoMB),
	})
	if err != nil {
		return nil, err
	}
	return envoy.NewHTTPSListener(config.HTTPSPortInternal, []*v3.FilterChain{filterChain}, cfg.EnableProxyProtocol)
}

func privateKeyProvider(mbEnabled bool) string {
	if mbEnabled {
		return "cryptomb"
	}
	return ""
}
