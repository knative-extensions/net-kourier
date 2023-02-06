//go:build e2e
// +build e2e

/*
Copyright 2023 The Knative Authors

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

package tls

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

const (
	// The cipher which is configured in config-network for test. See test/e2e-kind.sh.
	allowedCipher = tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256

	// The cipher which is NOT configured in config-network for test.
	disabledCipher = tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA

	// TLS handshake failure message which is implemented in Go TLS library.
	handshakeFailure = "handshake failure"
)

// TestCipherSuites verifies that tls client only works with the cipher suites configured.
func TestCipherSuites(t *testing.T) {
	t.Parallel()
	ctx, clients := context.Background(), test.Setup(t)

	tests := []struct {
		cipherSuites []uint16
		expErr       string
	}{{
		cipherSuites: []uint16{allowedCipher},
	}, {
		cipherSuites: []uint16{disabledCipher},
		expErr:       handshakeFailure,
	}}

	for _, test := range tests {
		host, client := create(ctx, t, clients, test.cipherSuites)
		err := sendRequest(ctx, t, client, host)
		if err != nil && !strings.Contains(err.Error(), test.expErr) {
			t.Fatalf("Unexpected error: %v, wanted %q", err, test.expErr)
		}
		if err == nil && test.expErr != "" {
			t.Fatalf("Failed to disallow cipher, wanted error: %q", test.expErr)
		}
	}
}

func create(ctx context.Context, t *testing.T, clients *test.Clients, cipherSuites []uint16) (string, *http.Client) {
	name, port, _ := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)
	hosts := []string{name + ".example.com"}

	secretName, tlsConfig, _ := ingress.CreateTLSSecret(ctx, t, clients, hosts)
	tlsConfig.CipherSuites = cipherSuites
	// TLS 1.3 ciphersuites are not configurable. See https://pkg.go.dev/crypto/tls#Config
	tlsConfig.MaxVersion = tls.VersionTLS12

	_, client, _ := ingress.CreateIngressReadyWithTLS(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts:      hosts,
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
		TLS: []v1alpha1.IngressTLS{{
			Hosts:           hosts,
			SecretName:      secretName,
			SecretNamespace: test.ServingNamespace,
		}},
	}, tlsConfig)
	return hosts[0], client
}

func sendRequest(ctx context.Context, t *testing.T, client *http.Client, hostname string) error {
	// Check without TLS.
	ingress.RuntimeRequest(ctx, t, client, "http://"+hostname)

	// Check with TLS.
	resp, err := client.Get("https://" + hostname)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Unexpected status code: %d, wanted %v", resp.StatusCode, http.StatusOK)
		ingress.DumpResponse(ctx, t, resp)
	}
	return nil
}
