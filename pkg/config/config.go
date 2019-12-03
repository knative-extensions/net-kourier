package config

const (
	HttpPortExternal  = uint32(8080)
	HttpPortInternal  = uint32(8081)
	HttpsPortExternal = uint32(8443)

	InternalKourierHeader = "kourier-snapshot-id"
	InternalKourierDomain = "internalkourier"
	InternalKourierPath   = "/__internalkouriersnapshot"

	KourierIngressClassName = "kourier.ingress.networking.knative.dev"

	EnvCertsSecretNamespace = "CERTS_SECRET_NAMESPACE"
	EnvCertsSecretName      = "CERTS_SECRET_NAME"
	CertFieldInSecret       = "tls.crt"
	KeyFieldInSecret        = "tls.key"
)
