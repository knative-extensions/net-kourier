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

package config

import (
	"os"

	"knative.dev/pkg/kmap"
	"knative.dev/pkg/network"
	"knative.dev/pkg/system"
)

const (
	// ControllerName is the name of the kourier controller.
	ControllerName = "net-kourier-controller"

	// InternalServiceName is the name of the internal service.
	InternalServiceName = "kourier-internal"

	// ExternalServiceName is the name of the external service.
	ExternalServiceName = "kourier"

	// HTTPPortExternal is the port for external availability.
	HTTPPortExternal = uint32(8080)

	// HTTPPortLocal is the port for internal availability.
	HTTPPortLocal = uint32(8081)

	// HTTPSPortLocal is the port for internal HTTPS availability.
	HTTPSPortLocal = uint32(8444)

	// HTTPSPortExternal is the port for external HTTPS availability.
	HTTPSPortExternal = uint32(8443)

	// HTTPPortProb is the port for prob
	HTTPPortProb = uint32(8090)

	// HTTPSPortProb is the port for prob
	HTTPSPortProb = uint32(9443)

	// InternalKourierDomain is an internal envoy endpoint.
	InternalKourierDomain = "internalkourier"

	// GatewayNamespaceEnv is an env variable specifying where the gateway is deployed.
	GatewayNamespaceEnv = "KOURIER_GATEWAY_NAMESPACE"

	// disableHTTP2AnnotationKey is the annotation key attached to a Knative Domain Mapping
	// to indicate that http2 should not be enabled for it.
	disableHTTP2AnnotationKey = "kourier.knative.dev/disable-http2"

	// trustedHopsCount Configure the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust.
	trustedHopsCount = "trusted-hops-count"

	// CipherSuites is the cipher suites for TLS external listener.
	cipherSuites = "cipher-suites"
)

// KourierIngressClassName is the class name to reconcile.
var KourierIngressClassName = GetIngressClassName()

var disableHTTP2Annotation = kmap.KeyPriority{
	disableHTTP2AnnotationKey,
}

// ServiceHostnames returns the external and internal service's respective hostname.
//
// Example: kourier.kourier-system.svc.cluster.local.
func ServiceHostnames() (string, string) {
	return network.GetServiceHostname(ExternalServiceName, GatewayNamespace()),
		network.GetServiceHostname(InternalServiceName, GatewayNamespace())
}

// GatewayNamespace returns the namespace where the gateway is deployed.
func GatewayNamespace() string {
	namespace := os.Getenv(GatewayNamespaceEnv)
	if namespace == "" {
		return system.Namespace()
	}
	return namespace
}

// GetDisableHTTP2 specifies whether http2 is going to be disabled
func GetDisableHTTP2(annotations map[string]string) (val string) {
	return disableHTTP2Annotation.Value(annotations)
}

func GetIngressClassName() string {
	defaultIngressClassName := "kourier.ingress.networking.knative.dev"
	className := os.Getenv("KOURIER_INGRESS_CLASS_NAME")
	if className != "" {
		return className
	}
	return defaultIngressClassName
}
