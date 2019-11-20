package config

import (
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
)

const (
	HttpPortExternal  = uint32(8080)
	HttpPortInternal  = uint32(8081)
	HttpsPortExternal = uint32(8443)

	InternalKourierHeader = "kourier-snapshot-id"
	InternalKourierDomain = "internalkourier"
	InternalKourierPath   = "/__internalkouriersnapshot"

	KourierNamespaceEnv     = "KOURIER_NAMESPACE"
	KourierDefaultNamespace = "knative-serving"
)

var kourierNamespace string
var once sync.Once

func Namespace() string {
	once.Do(
		func() {
			kourierNamespace = os.Getenv(KourierNamespaceEnv)

			if kourierNamespace == "" {
				log.Infof(
					"Env %s empty, using default: %s",
					KourierNamespaceEnv,
					KourierDefaultNamespace,
				)

				kourierNamespace = KourierDefaultNamespace
			}
		},
	)

	return kourierNamespace
}
