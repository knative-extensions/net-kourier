//go:build e2e
// +build e2e

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

package extauthz

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"

	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

func TestExtAuthz(t *testing.T) {
	clients := test.Setup(t)
	ctx := context.Background()

	name, port, _ := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
	_, client, _ := ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts:      []string{name + ".example.com"},
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
	})

	req, err := http.NewRequest("GET", "http://"+name+".example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusForbidden)

	// Prepare the request, this one should succeed
	req, err = http.NewRequest("GET", "http://"+name+".example.com/success", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Do the request
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	// When POSTing binary data with a gRPC ext-authz, without KOURIER_EXTAUTHZ_PACKASBYTES, the result is "Forbidden"
	// Because data passed to ext-authz gRPC service cannot be serialized. See https://github.com/knative-sandbox/net-kourier/issues/830
	req, err = http.NewRequest("POST", "http://"+name+".example.com/success", bytes.NewReader([]byte{0x04, 0xf1}))
	if err != nil {
		t.Fatal(err)
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if os.Getenv("KOURIER_EXTAUTHZ_PROTOCOL") == "grpc" && os.Getenv("KOURIER_EXTAUTHZ_PACKASBYTES") == "" {
		assert.Equal(t, resp.StatusCode, http.StatusForbidden)
	} else {
		assert.Equal(t, resp.StatusCode, http.StatusOK)
	}
}
