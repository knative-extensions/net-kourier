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
)
