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
	"fmt"
	"os"

	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/envoy"

	"go.uber.org/zap"

	httpconnmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

const (
	envCertsSecretNamespace = "CERTS_SECRET_NAMESPACE"
	envCertsSecretName      = "CERTS_SECRET_NAME"
	certFieldInSecret       = "tls.crt"
	keyFieldInSecret        = "tls.key"
	externalRouteConfigName = "external_services"
	internalRouteConfigName = "internal_services"
)

// For now, when updating the info for an ingress we delete it, and then
// regenerate it. We can optimize this later.
func UpdateInfoForIngress(caches *Caches, ingress *v1alpha1.Ingress, kubeclient kubeclient.Interface, translator *IngressTranslator, logger *zap.SugaredLogger, extAuthzEnabled bool) error {
	logger.Infof("Updating Knative Ingress %s/%s", ingress.Name, ingress.Namespace)

	// Adds a header with the ingress Hash and a random value header to force the config reload.
	err := InsertKourierHeaders(ingress)
	if err != nil {
		return fmt.Errorf("failed to add knative probe header in ingress: %s", ingress.GetName())
	}

	ingressTranslation, err := translator.translateIngress(ingress, extAuthzEnabled)
	if err != nil {
		return err
	}

	if ingressTranslation == nil {
		return nil
	}

	return caches.UpdateIngress(ingress, ingressTranslation, kubeclient)
}

func listenersFromVirtualHosts(externalVirtualHosts []*route.VirtualHost,
	clusterLocalVirtualHosts []*route.VirtualHost,
	sniMatches []*envoy.SNIMatch,
	kubeclient kubeclient.Interface, caches *Caches) ([]*v2.Listener, error) {

	var listeners []*v2.Listener

	externalManager := envoy.NewHTTPConnectionManager(externalVirtualHosts)
	internalManager := envoy.NewHTTPConnectionManager(clusterLocalVirtualHosts)

	internalRouteConfig := internalManager.GetRouteConfig()
	externalRouteConfig := externalManager.GetRouteConfig()

	// We need to keep these for the case of HTTPS with SNI routing
	originalExternalManager := externalManager
	originalExternalVHosts := externalRouteConfig.VirtualHosts

	// Set proper names so those can be referred later.
	internalRouteConfig.Name = internalRouteConfigName
	externalRouteConfig.Name = externalRouteConfigName

	// Now we save the RouteConfigs with the proper name and all the virtualhosts etc.. into the cache.
	caches.routeConfig = []v2.RouteConfiguration{}
	caches.routeConfig = append(caches.routeConfig, *externalRouteConfig)
	caches.routeConfig = append(caches.routeConfig, *internalRouteConfig)

	// Now let's forget about the cache, and override the internal manager to point to the RDS and look for the proper
	// names.
	internalRDSHTTPConnectionManager := envoy.NewRDSHTTPConnectionManager(internalRouteConfigName)
	internalManager.RouteSpecifier = &internalRDSHTTPConnectionManager

	// Set the discovery to ADS
	externalRDSHTTPConnectionManager := envoy.NewRDSHTTPConnectionManager(externalRouteConfigName)
	externalManager.RouteSpecifier = &externalRDSHTTPConnectionManager

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

func newExternalEnvoyListenerWithOneCert(manager *httpconnmanagerv2.HttpConnectionManager, kubeClient kubeclient.Interface) (*v2.Listener, error) {
	certificateChain, privateKey, err := sslCreds(
		kubeClient, os.Getenv(envCertsSecretNamespace), os.Getenv(envCertsSecretName),
	)

	if err != nil {
		return nil, err
	}

	return envoy.NewHTTPSListener(manager, config.HTTPSPortExternal, certificateChain, privateKey)
}

func newExternalHTTPEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager) (*v2.Listener, error) {
	return envoy.NewHTTPListener(manager, config.HTTPPortExternal)
}

func newInternalEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager) (*v2.Listener, error) {
	return envoy.NewHTTPListener(manager, config.HTTPPortInternal)
}

func newExternalHTTPSEnvoyListener(manager *httpconnmanagerv2.HttpConnectionManager, sniMatches []*envoy.SNIMatch) (*v2.Listener, error) {
	return envoy.NewHTTPSListenerWithSNI(manager, config.HTTPSPortExternal, sniMatches)
}
