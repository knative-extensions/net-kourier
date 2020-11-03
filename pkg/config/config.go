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

	"knative.dev/pkg/network"
	"knative.dev/pkg/system"
)

const (
	ControllerName = "kourier"

	EnvoyNodeID = "3scale-kourier-gateway"

	InternalServiceName = "kourier-internal"
	ExternalServiceName = "kourier"

	HTTPPortExternal  = uint32(8080)
	HTTPPortInternal  = uint32(8081)
	HTTPSPortExternal = uint32(8443)

	InternalKourierDomain = "internalkourier"

	GatewayNamespaceEnv = "KOURIER_GATEWAY_NAMESPACE"

	KourierIngressClassName = "kourier.ingress.networking.knative.dev"
)

// ServiceDomains returns the external and internal service's respective domain.
//
// Example: kourier.kourier-system.svc.cluster.local.
func ServiceDomains() (string, string) {
	suffix := "." + GatewayNamespace() + ".svc." + network.GetClusterDomainName()
	return ExternalServiceName + suffix, InternalServiceName + suffix
}

func GatewayNamespace() string {
	namespace := os.Getenv(GatewayNamespaceEnv)
	if namespace == "" {
		return system.Namespace()
	}
	return namespace
}
