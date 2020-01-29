package config

const (
	HTTPPortExternal  = uint32(8080)
	HTTPPortInternal  = uint32(8081)
	HTTPSPortExternal = uint32(8443)

	InternalKourierDomain = "internalkourier"
	InternalKourierPath   = "/__internalkouriersnapshot"

	KourierIngressClassName = "kourier.ingress.networking.knative.dev"
)
