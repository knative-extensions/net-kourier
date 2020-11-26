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

package ha

import (
	"context"
	"net/url"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	pkgTest "knative.dev/pkg/test"
	"knative.dev/pkg/test/spoof"
)

const (
	haReplicas = 2
	domain     = ".example.com"
)

func createIngressSpec(name string, port int) v1alpha1.IngressSpec {
	return v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts: []string{name + domain},
			HTTP: &v1alpha1.HTTPIngressRuleValue{
				Paths: []v1alpha1.HTTPIngressPath{{
					Splits: []v1alpha1.IngressBackendSplit{{
						IngressBackend: v1alpha1.IngressBackend{
							ServiceName:      name,
							ServiceNamespace: test.ServingNamespace,
							ServicePort:      intstr.FromInt(port),
						},
					}},
				}},
			},
			Visibility: v1alpha1.IngressVisibilityExternalIP,
		}},
	}
}

func assertIngressEventuallyWorks(ctx context.Context, t *testing.T, clients *test.Clients, url *url.URL) {
	t.Helper()
	if _, err := pkgTest.WaitForEndpointState(
		ctx,
		clients.KubeClient,
		t.Logf,
		url,
		spoof.IsStatusOK,
		"WaitForIngressToReturnSuccess",
		test.NetworkingFlags.ResolvableDomain); err != nil {
		t.Fatalf("The service at %s didn't return success: %v", url, err)
	}
}
