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
	"os"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	httpconnmanagerv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/api"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/ingress"
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
func UpdateInfoForIngress(ctx context.Context, caches *Caches, ing *v1alpha1.Ingress, translator *IngressTranslator, extAuthzEnabled bool) error {
	// Adds a header with the ingress Hash and a random value header to force the config reload.
	_, err := ingress.InsertProbe(ing)
	if err != nil {
		return fmt.Errorf("failed to add knative probe header in ingress: %s", ing.GetName())
	}

	ingressTranslation, err := translator.translateIngress(ctx, ing, extAuthzEnabled)
	if err != nil {
		return err
	}

	if ingressTranslation == nil {
		return nil
	}

	return caches.UpdateIngress(ctx, ingressTranslation)
}

func listenersFromVirtualHosts(
	ctx context.Context,
	externalVirtualHosts []*route.VirtualHost,
	clusterLocalVirtualHosts []*route.VirtualHost,
	sniMatches []*envoy.SNIMatch,
	kubeclient kubeclient.Interface,
	caches *Caches) ([]*v2.Listener, error) {

	// First, we save the RouteConfigs with the proper name and all the virtualhosts etc. into the cache.
	externalRouteConfig := envoy.NewRouteConfig(externalRouteConfigName, externalVirtualHosts)
	internalRouteConfig := envoy.NewRouteConfig(internalRouteConfigName, clusterLocalVirtualHosts)
	caches.routeConfig = []*v2.RouteConfiguration{externalRouteConfig, internalRouteConfig}

	// Now we setup connection managers, that reference the routeconfigs via RDS.
	externalManager := envoy.NewHTTPConnectionManager()
	internalManager := envoy.NewHTTPConnectionManager()
	externalManager.RouteSpecifier = envoy.NewRDSHTTPConnectionManager(externalRouteConfig.Name)
	internalManager.RouteSpecifier = envoy.NewRDSHTTPConnectionManager(internalRouteConfig.Name)
	externalHTTPEnvoyListener, err := envoy.NewHTTPListener(externalManager, config.HTTPPortExternal)
	if err != nil {
		return nil, err
	}
	internalEnvoyListener, err := envoy.NewHTTPListener(internalManager, config.HTTPPortInternal)
	if err != nil {
		return nil, err
	}

	listeners := []*v2.Listener{externalHTTPEnvoyListener, internalEnvoyListener}

	// Configure TLS Listener. If there's at least one ingress that contains the
	// TLS field, that takes precedence. If there is not, TLS will be configured
	// using a single cert for all the services if the creds are given via ENV.
	if len(sniMatches) > 0 {
		externalHTTPSEnvoyListener, err := envoy.NewHTTPSListenerWithSNI(externalManager, config.HTTPSPortExternal, sniMatches)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, externalHTTPSEnvoyListener)
	} else if useHTTPSListenerWithOneCert() {
		externalHTTPSEnvoyListener, err := newExternalEnvoyListenerWithOneCert(
			ctx, externalManager, kubeclient,
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

func sslCreds(ctx context.Context, kubeClient kubeclient.Interface, secretNamespace string, secretName string) (certificateChain []byte, privateKey []byte, err error) {
	secret, err := kubeClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	return secret.Data[certFieldInSecret], secret.Data[keyFieldInSecret], nil
}

func newExternalEnvoyListenerWithOneCert(ctx context.Context, manager *httpconnmanagerv2.HttpConnectionManager, kubeClient kubeclient.Interface) (*v2.Listener, error) {
	certificateChain, privateKey, err := sslCreds(
		ctx, kubeClient, os.Getenv(envCertsSecretNamespace), os.Getenv(envCertsSecretName),
	)
	if err != nil {
		return nil, err
	}

	return envoy.NewHTTPSListener(manager, config.HTTPSPortExternal, certificateChain, privateKey)
}
