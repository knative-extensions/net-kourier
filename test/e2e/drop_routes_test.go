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

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"knative.dev/net-kourier/pkg/config"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

func TestDropRoutes(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)

	name, port, _ := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	ing, client, _ := ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
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

	// testing access to /favicon.ico -- should have access
	req, err := http.NewRequest("GET", "http://"+name+".example.com/favicon.ico", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	assert.NilError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	// blocking access to /favicon.ico
	oldData, err := json.Marshal(ing)
	if err != nil {
		t.Fatal(err)
	}
	dropRoutesAnnotationValue := `{"routes":[{"path":"/favicon.ico"}]}`
	ing.Annotations[config.DropRoutesAnnotationKey] = dropRoutesAnnotationValue
	newData, err := json.Marshal(ing)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, ing)
	if err != nil {
		t.Fatal(err)
	}
	_, err = clients.NetworkingClient.Ingresses.Patch(ctx, ing.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// wait for the new config to be applied
	time.Sleep(time.Second)

	// testing access to /favicon.ico -- should not have access
	req, err = http.NewRequest("GET", "http://"+name+".example.com/favicon.ico", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err = client.Do(req)
	assert.NilError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusNotFound)
}
