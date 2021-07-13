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
	"flag"
	"os"

	"knative.dev/net-kourier/pkg/config"
	kourierIngressController "knative.dev/net-kourier/pkg/reconciler/ingress"

	// This defines the shared main for injected controllers.
	"knative.dev/pkg/injection/sharedmain"
)

var (
	probeAddr = flag.String("probe-addr", "", "run this binary as a health check against the given address")
)

func main() {
	flag.Parse()

	// Run the binary as a health checker if the respective flag is given.
	if *probeAddr != "" {
		os.Exit(check(*probeAddr))
	}

	sharedmain.Main(config.ControllerName, kourierIngressController.NewController)
}
