package generator

import (
	"fmt"
	"kourier/pkg/config"
	"kourier/pkg/envoy"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	log "github.com/sirupsen/logrus"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/pkg/reconciler/ingress/resources"
)

// Generates an internal virtual host that signals that the Envoy instance has
// been configured for all the ingresses received in the params.
// The virtual host generated contains a route for each ingress received in the
// params. The path of the routes are hashed ingresses. With this, if the
// request for a hashed ingress is successful, we know that the gateway has been
// configured for that ingress.
func statusVHost(ingresses []*v1alpha1.Ingress) route.VirtualHost {
	return envoy.NewVirtualHost(
		config.InternalKourierDomain,
		[]string{config.InternalKourierDomain},
		statusRoutes(ingresses),
	)
}

func statusRoutes(ingresses []*v1alpha1.Ingress) []*route.Route {
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
		routes = append(routes, envoy.NewRouteStatusOK(name, path))
	}

	staticRoute := envoy.NewRouteStatusOK(
		config.InternalKourierDomain,
		config.InternalKourierPath,
	)
	routes = append(routes, staticRoute)

	return routes
}
