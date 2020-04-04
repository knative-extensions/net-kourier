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

const (
	InternalServiceName = "kourier-internal"
	ExternalServiceName = "kourier"

	HTTPPortExternal  = uint32(8080)
	HTTPPortInternal  = uint32(8081)
	HTTPSPortExternal = uint32(8443)

	InternalKourierDomain    = "internalkourier"
	InternalKourierPath      = "/__internalkouriersnapshot"
	ExtAuthzHostEnv          = "KOURIER_EXTAUTHZ_HOST"
	ExtAuthzFailureModeEnv   = "KOURIER_EXTAUTHZ_FAILUREMODEALLOW"
	ExtAuthzMaxRequestsBytes = "KOURIER_EXTAUTHZ_MAXREQUESTBYTES"
	ExtAuthzTimeout          = "KOURIER_EXTAUTHZ_TIMEOUT"
	ExternalAuthzCluster     = "extAuthz"

	KourierIngressClassName = "kourier.ingress.networking.knative.dev"
	// Hack to force envoy reload
	KourierHeaderRandom = "Kourier-random-header"
)
