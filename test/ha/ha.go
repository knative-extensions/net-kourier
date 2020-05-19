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
	"testing"

	"net/url"

	"k8s.io/apimachinery/pkg/util/intstr"
	pkgTest "knative.dev/pkg/test"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
	"knative.dev/serving/test"
)

const (
	haReplicas = 2
	domain     = ".example.com"
)

func createIngressSpec(name string, port int) v1alpha1.IngressSpec {
	return v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts:      []string{name + domain},
			Visibility: v1alpha1.IngressVisibilityExternalIP,
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
		}},
	}
}

func assertIngressEventuallyWorks(t *testing.T, clients *test.Clients, url *url.URL) {
	t.Helper()
	if _, err := pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		url,
		pkgTest.IsStatusOK,
		"WaitForIngressToReturnSuccess",
		test.ServingFlags.ResolvableDomain); err != nil {
		t.Fatalf("The service at %s didn't return success: %v", url, err)
	}
}
