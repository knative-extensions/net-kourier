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
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
	"knative.dev/networking/pkg/certificates"
	"knative.dev/pkg/system"
)

const (
	externalRouteConfigName    = "external_services"
	externalTLSRouteConfigName = "external_tls_services"
	localRouteConfigName       = "internal_services"
	localTLSRouteConfigName    = "internal_tls_services"
)

// ErrDomainConflict is an error produces when two ingresses have conflicting domains.
var ErrDomainConflict = errors.New("ingress has a conflicting domain with another ingress")

type Caches struct {
	mu                  sync.Mutex
	translatedIngresses map[types.NamespacedName]*translatedIngress
	clusters            *ClustersCache
	domainsInUse        sets.Set[string]
	statusVirtualHost   *route.VirtualHost

	kubeClient kubeclient.Interface
}

func NewCaches(ctx context.Context, kubernetesClient kubeclient.Interface) (*Caches, error) {
	c := &Caches{
		translatedIngresses: make(map[types.NamespacedName]*translatedIngress),
		clusters:            newClustersCache(),
		domainsInUse:        sets.New[string](),
		statusVirtualHost:   statusVHost(),
		kubeClient:          kubernetesClient,
	}

	if config.FromContext(ctx).Kourier.ExternalAuthz.Enabled {
		c.clusters.set(config.FromContext(ctx).Kourier.ExternalAuthz.Cluster(), "__extAuthZCluster", "_internal")
	}
	return c, nil
}

func (caches *Caches) UpdateIngress(_ context.Context, ingressTranslation *translatedIngress) error {
	// we hold a lock for Updating the ingress, to avoid another worker to generate an snapshot just when we have
	// deleted the ingress before adding it.
	caches.mu.Lock()
	defer caches.mu.Unlock()

	caches.deleteTranslatedIngress(ingressTranslation.name.Name, ingressTranslation.name.Namespace)
	return caches.addTranslatedIngress(ingressTranslation)
}

func (caches *Caches) validateIngress(translatedIngress *translatedIngress) error {
	for _, vhost := range translatedIngress.localVirtualHosts {
		if caches.domainsInUse.HasAny(vhost.GetDomains()...) {
			return ErrDomainConflict
		}
	}

	return nil
}

func (caches *Caches) addTranslatedIngress(translatedIngress *translatedIngress) error {
	if err := caches.validateIngress(translatedIngress); err != nil {
		return err
	}

	for _, vhost := range translatedIngress.localVirtualHosts {
		caches.domainsInUse.Insert(vhost.GetDomains()...)
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
	localTLSVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses)+1)
	externalVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses))
	externalTLSVHosts := make([]*route.VirtualHost, 0, len(caches.translatedIngresses))
	localSNIs := sniMatches{}
	externalSNIs := sniMatches{}

	for _, translatedIngress := range caches.translatedIngresses {
		localVHosts = append(localVHosts, translatedIngress.localVirtualHosts...)
		localTLSVHosts = append(localTLSVHosts, translatedIngress.localTLSVirtualHosts...)
		externalVHosts = append(externalVHosts, translatedIngress.externalVirtualHosts...)
		externalTLSVHosts = append(externalTLSVHosts, translatedIngress.externalTLSVirtualHosts...)

		for _, match := range translatedIngress.localSNIMatches {
			localSNIs.consume(match)
		}
		for _, match := range translatedIngress.externalSNIMatches {
			externalSNIs.consume(match)
		}
	}

	// Append the statusHost too.
	localVHosts = append(localVHosts, caches.statusVirtualHost)

	listeners, routes, clusters, err := generateListenersAndRouteConfigsAndClusters(
		ctx,
		externalVHosts,
		externalTLSVHosts,
		localVHosts,
		localTLSVHosts,
		localSNIs.list(),
		externalSNIs.list(),
		caches.kubeClient,
	)
	if err != nil {
		return nil, err
	}

	clusters = append(caches.clusters.list(), clusters...)

	return cache.NewSnapshot(
		uuid.NewString(),
		map[resource.Type][]cachetypes.Resource{
			resource.ClusterType:  clusters,
			resource.RouteType:    routes,
			resource.ListenerType: listeners,
		},
	)
}

// DeleteIngressInfo removes an ingress from the caches.
//
// Notice that the clusters are not deleted. That's handled with the expiration
// time set in the "ClustersCache" struct.
func (caches *Caches) DeleteIngressInfo(_ context.Context, ingressName string, ingressNamespace string) error {
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
			caches.clusters.setExpiration(cluster.GetName(), ingressName, ingressNamespace)
		}

		for _, vhost := range translated.localVirtualHosts {
			caches.domainsInUse.Delete(vhost.GetDomains()...)
		}

		delete(caches.translatedIngresses, key)
	}
}

func generateListenersAndRouteConfigsAndClusters(
	ctx context.Context,
	externalVirtualHosts []*route.VirtualHost,
	externalTLSVirtualHosts []*route.VirtualHost,
	localVirtualHosts []*route.VirtualHost,
	localTLSVirtualHosts []*route.VirtualHost,
	localSNIMatches []*envoy.SNIMatch,
	externalSNIMatches []*envoy.SNIMatch,
	kubeclient kubeclient.Interface,
) ([]cachetypes.Resource, []cachetypes.Resource, []cachetypes.Resource, error) {
	// This has to be "OrDefaults" because this path is called before the informers are
	// running when booting the controller up and prefilling the config before making it
	// ready.
	cfg := config.FromContextOrDefaults(ctx)

	// First, we save the RouteConfigs with the proper name and all the virtualhosts etc. into the cache.
	externalRouteConfig := envoy.NewRouteConfig(externalRouteConfigName, externalVirtualHosts)
	externalTLSRouteConfig := envoy.NewRouteConfig(externalTLSRouteConfigName, externalTLSVirtualHosts)
	localRouteConfig := envoy.NewRouteConfig(localRouteConfigName, localVirtualHosts)

	// Now we setup connection managers, that reference the routeconfigs via RDS.
	externalManager := envoy.NewHTTPConnectionManager(externalRouteConfig.GetName(), cfg.Kourier)
	externalTLSManager := envoy.NewHTTPConnectionManager(externalTLSRouteConfig.GetName(), cfg.Kourier)
	localManager := envoy.NewHTTPConnectionManager(localRouteConfig.GetName(), cfg.Kourier)

	externalHTTPEnvoyListener, err := envoy.NewHTTPListener(externalManager, config.HTTPPortExternal, cfg.Kourier.EnableProxyProtocol, cfg.Kourier.ListenIPAddresses)
	if err != nil {
		return nil, nil, nil, err
	}
	localEnvoyListener, err := envoy.NewHTTPListener(localManager, config.HTTPPortLocal, false, cfg.Kourier.ListenIPAddresses)
	if err != nil {
		return nil, nil, nil, err
	}

	listeners := []cachetypes.Resource{externalHTTPEnvoyListener, localEnvoyListener}
	routes := []cachetypes.Resource{externalRouteConfig, localRouteConfig}
	clusters := make([]cachetypes.Resource, 0, 1)

	// create probe listeners
	probHTTPListener, err := envoy.NewHTTPListener(externalManager, config.HTTPPortProb, false, cfg.Kourier.ListenIPAddresses)
	if err != nil {
		return nil, nil, nil, err
	}
	listeners = append(listeners, probHTTPListener)

	// Configure TLS listener for cluster-local traffic.
	// If there's at least one ingress that contains the TLS field, that takes precedence.
	// If there is not, TLS will be configured using a single cert for all the services when the certificate is configured.
	if len(localSNIMatches) > 0 {
		localTLSRouteConfig := envoy.NewRouteConfig(localTLSRouteConfigName, localTLSVirtualHosts)
		localTLSManager := envoy.NewHTTPConnectionManager(localTLSRouteConfig.GetName(), cfg.Kourier)

		localHTTPSEnvoyListener, err := envoy.NewHTTPSListenerWithSNI(
			localTLSManager, config.HTTPSPortLocal,
			localSNIMatches, cfg.Kourier,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		probeConfig := cfg.Kourier
		probeConfig.EnableProxyProtocol = false // Disable proxy protocol for prober.

		// create https prob listener with SNI
		probHTTPSListener, err := envoy.NewHTTPSListenerWithSNI(
			localManager, config.HTTPSPortProb,
			localSNIMatches, probeConfig,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		// if a single certificate is additionally configured, add a new filter chain to TLS listener
		if cfg.Kourier.ClusterCertSecret != "" {
			localHTTPSEnvoyListenerWithOneCertFilterChain, err := newLocalEnvoyListenerWithOneCertFilterChain(
				ctx, localTLSManager, kubeclient, cfg.Kourier,
			)
			if err != nil {
				return nil, nil, nil, err
			}

			localHTTPSEnvoyListener.FilterChains = append(localHTTPSEnvoyListener.FilterChains,
				localHTTPSEnvoyListenerWithOneCertFilterChain)
			probHTTPSListener.FilterChains = append(probHTTPSListener.FilterChains,
				localHTTPSEnvoyListenerWithOneCertFilterChain)
		}

		listeners = append(listeners, localHTTPSEnvoyListener, probHTTPSListener)
		routes = append(routes, localTLSRouteConfig)
	} else if cfg.Kourier.ClusterCertSecret != "" {
		localTLSRouteConfig := envoy.NewRouteConfig(localTLSRouteConfigName, localVirtualHosts)
		localTLSManager := envoy.NewHTTPConnectionManager(localTLSRouteConfig.GetName(), cfg.Kourier)

		localHTTPSEnvoyListener, err := newLocalEnvoyListenerWithOneCert(
			ctx, localTLSManager, kubeclient,
			cfg.Kourier,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		listeners = append(listeners, localHTTPSEnvoyListener)
		routes = append(routes, localTLSRouteConfig)
	}

	// Configure TLS Listener. If there's at least one ingress that contains the
	// TLS field, that takes precedence. If there is not, TLS will be configured
	// using a single cert for all the services if the creds are given via ENV.
	if len(externalSNIMatches) > 0 {
		externalHTTPSEnvoyListener, err := envoy.NewHTTPSListenerWithSNI(
			externalTLSManager, config.HTTPSPortExternal,
			externalSNIMatches, cfg.Kourier,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		probeConfig := cfg.Kourier
		probeConfig.EnableProxyProtocol = false // Disable proxy protocol for prober.

		// create https prob listener with SNI
		probHTTPSListener, err := envoy.NewHTTPSListenerWithSNI(
			externalManager, config.HTTPSPortProb,
			externalSNIMatches, probeConfig,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		// if a single certificate is additionally configured, add a new filter chain to TLS listener
		if cfg.Kourier.UseHTTPSListenerWithOneCert() {
			externalHTTPSEnvoyListenerWithOneCertFilterChain, err := newExternalEnvoyListenerWithOneCertFilterChain(
				ctx, externalTLSManager, kubeclient, cfg.Kourier,
			)
			if err != nil {
				return nil, nil, nil, err
			}

			externalHTTPSEnvoyListener.FilterChains = append(externalHTTPSEnvoyListener.FilterChains,
				externalHTTPSEnvoyListenerWithOneCertFilterChain)
			probHTTPSListener.FilterChains = append(probHTTPSListener.FilterChains,
				externalHTTPSEnvoyListenerWithOneCertFilterChain)
		}

		listeners = append(listeners, externalHTTPSEnvoyListener, probHTTPSListener)
		routes = append(routes, externalTLSRouteConfig)
	} else if cfg.Kourier.UseHTTPSListenerWithOneCert() {
		externalHTTPSEnvoyListener, err := newExternalEnvoyListenerWithOneCert(
			ctx, externalTLSManager, kubeclient,
			cfg.Kourier,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		// create https prob listener
		probHTTPSListener, err := envoy.NewHTTPSListener(config.HTTPSPortProb, externalHTTPSEnvoyListener.GetFilterChains(), false, cfg.Kourier.ListenIPAddresses)
		if err != nil {
			return nil, nil, nil, err
		}

		listeners = append(listeners, externalHTTPSEnvoyListener, probHTTPSListener)
		routes = append(routes, externalTLSRouteConfig)
	}

	if cluster := cfg.Kourier.Tracing.Cluster(); cluster != nil {
		clusters = append(clusters, cluster)
	}

	return listeners, routes, clusters, nil
}

func sslCreds(ctx context.Context, kubeClient kubeclient.Interface, secretNamespace string, secretName string) (certificateChain []byte, privateKey []byte, err error) {
	secret, err := kubeClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	return secret.Data[certificates.CertName], secret.Data[certificates.PrivateKeyName], nil
}

func newExternalEnvoyListenerWithOneCertFilterChain(ctx context.Context, manager *httpconnmanagerv3.HttpConnectionManager, kubeClient kubeclient.Interface, cfg *config.Kourier) (*v3.FilterChain, error) {
	certificateChain, privateKey, err := sslCreds(
		ctx, kubeClient, cfg.CertsSecretNamespace, cfg.CertsSecretName,
	)
	if err != nil {
		return nil, err
	}

	return envoy.CreateFilterChainFromCertificateAndPrivateKey(manager, &envoy.Certificate{
		Certificate:        certificateChain,
		PrivateKey:         privateKey,
		PrivateKeyProvider: privateKeyProvider(cfg.EnableCryptoMB),
		CipherSuites:       sets.List(cfg.CipherSuites),
	})
}

func newExternalEnvoyListenerWithOneCert(ctx context.Context, manager *httpconnmanagerv3.HttpConnectionManager, kubeClient kubeclient.Interface, cfg *config.Kourier) (*v3.Listener, error) {
	filterChain, err := newExternalEnvoyListenerWithOneCertFilterChain(ctx, manager, kubeClient, cfg)
	if err != nil {
		return nil, err
	}

	return envoy.NewHTTPSListener(config.HTTPSPortExternal, []*v3.FilterChain{filterChain}, cfg.EnableProxyProtocol, cfg.ListenIPAddresses)
}

func newLocalEnvoyListenerWithOneCertFilterChain(ctx context.Context, manager *httpconnmanagerv3.HttpConnectionManager, kubeClient kubeclient.Interface, cfg *config.Kourier) (*v3.FilterChain, error) {
	certificateChain, privateKey, err := sslCreds(ctx, kubeClient, system.Namespace(), cfg.ClusterCertSecret)
	if err != nil {
		return nil, err
	}
	return envoy.CreateFilterChainFromCertificateAndPrivateKey(manager, &envoy.Certificate{
		Certificate:        certificateChain,
		PrivateKey:         privateKey,
		PrivateKeyProvider: privateKeyProvider(cfg.EnableCryptoMB),
		CipherSuites:       sets.List(cfg.CipherSuites),
	})
}

func newLocalEnvoyListenerWithOneCert(ctx context.Context, manager *httpconnmanagerv3.HttpConnectionManager, kubeClient kubeclient.Interface, cfg *config.Kourier) (*v3.Listener, error) {
	filterChain, err := newLocalEnvoyListenerWithOneCertFilterChain(ctx, manager, kubeClient, cfg)
	if err != nil {
		return nil, err
	}
	return envoy.NewHTTPSListener(config.HTTPSPortLocal, []*v3.FilterChain{filterChain}, cfg.EnableProxyProtocol, cfg.ListenIPAddresses)
}

func privateKeyProvider(mbEnabled bool) string {
	if mbEnabled {
		return "cryptomb"
	}
	return ""
}
