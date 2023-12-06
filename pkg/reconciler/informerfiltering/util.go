/*
Copyright 2022 The Knative Authors

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

package informerfiltering

import (
	"context"
	"os"
	"strconv"

	"knative.dev/networking/pkg/apis/networking"
	filteredFactory "knative.dev/pkg/client/injection/kube/informers/factory/filtered"
)

// TODO: rle extract this to pkg
const KnativeCABundleLabelKey = "knative-ca-trust-bundle"

const EnableSecretInformerFilteringByCertUIDEnv = "ENABLE_SECRET_INFORMER_FILTERING_BY_CERT_UID"

// ShouldFilterByCertificateUID allows to choose whether to apply filtering on certificate related secrets
// when list by informers in this component. If not set or set to false no filtering is applied and instead informers
// will get any secret available in the cluster which may lead to mem issues in large clusters.
func ShouldFilterByCertificateUID() bool {
	if enable := os.Getenv(EnableSecretInformerFilteringByCertUIDEnv); enable != "" {
		b, _ := strconv.ParseBool(enable)
		return b
	}
	return false
}

// GetContextWithFilteringLabelSelector returns the passed context with the proper label key selector added to it.
func GetContextWithFilteringLabelSelector(ctx context.Context) context.Context {
	if ShouldFilterByCertificateUID() {
		return filteredFactory.WithSelectors(ctx, networking.CertificateUIDLabelKey)
	}
	return filteredFactory.WithSelectors(ctx, "") // Allow all
}
