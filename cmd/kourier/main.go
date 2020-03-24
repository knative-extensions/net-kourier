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

package main

import (
	"os"

	kourierIngressController "knative.dev/net-kourier/pkg/reconciler/ingress"

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
