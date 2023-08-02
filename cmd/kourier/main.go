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
	"knative.dev/net-kourier/pkg/config"
	kourierIngressController "knative.dev/net-kourier/pkg/reconciler/ingress"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/pkg/signals"

	// This defines the shared main for injected controllers.
	filteredFactory "knative.dev/pkg/client/injection/kube/informers/factory/filtered"
	"knative.dev/pkg/injection/sharedmain"
)

func main() {
	ctx := filteredFactory.WithSelectors(signals.NewContext(), networking.CertificateUIDLabelKey)
	ctx = sharedmain.WithHealthProbesDisabled(ctx)
	sharedmain.MainWithContext(ctx, config.ControllerName, kourierIngressController.NewController)
}
