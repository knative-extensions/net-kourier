package main

import (
	kourierIngressController "kourier/pkg/reconciler/ingress"
	"os"

	"knative.dev/pkg/controller"

	log "github.com/sirupsen/logrus"

	// This defines the shared main for injected controllers.
	"knative.dev/pkg/injection/sharedmain"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
}

func main() {
	// TODO: make this configurable
	_ = os.Setenv("METRICS_DOMAIN", "knative.dev/samples")

	// The controller by defaults uses 2 threads, but our reconcile logic doesn't support it yet
	// TODO: Improve reconcile to support multiple threads
	controller.DefaultThreadsPerController = 1

	sharedmain.Main("KourierIngressController", kourierIngressController.NewController)
}
