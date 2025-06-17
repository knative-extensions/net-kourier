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

package proxyprotocol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
	"knative.dev/pkg/network"
	"knative.dev/pkg/reconciler"
	pkgTest "knative.dev/pkg/test"
)

// TestProxyProtocol verifies that the kourier is configured with proxy protocol.
func TestProxyProtocol(t *testing.T) {
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

	// testing without proxy protocol headers
	req, err := http.NewRequest(http.MethodGet, "http://"+name+".example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	assert.Check(t, resp == nil)
	assert.Check(t, err != nil)

	// testing with proxy protocol
	client.Transport = &http.Transport{
		DialContext: createDialContextProxyProtocol(ctx, t, ing, clients),
	}

	resp, err = client.Do(req)
	assert.Check(t, err == nil)
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

// TestProxyProtocolWithSNI verifies that the kourier is configured with proxy protocol and SNI.
func TestProxyProtocolWithSNI(t *testing.T) {
	ctx, clients := context.Background(), test.Setup(t)
	name, port, _ := ingress.CreateRuntimeService(ctx, t, clients, networking.ServicePortNameHTTP1)

	privateServiceName := test.ObjectNameForTest(t)
	privateHostName := privateServiceName + "." + test.ServingNamespace + ".svc." + test.NetworkingFlags.ClusterSuffix

	privateIng, _, _ := ingress.CreateIngressReady(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts:      []string{privateHostName},
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

	// Slap an ExternalName service in front of the kingress
	loadbalancerAddress := privateIng.Status.PrivateLoadBalancer.Ingress[0].DomainInternal
	createExternalNameService(ctx, t, clients, privateHostName, loadbalancerAddress)

	// Using fixed hostnames can lead to conflicts when -count=N>1
	// so pseudo-randomize the hostnames to avoid conflicts.
	publicHostname := name + ".toto.custom.domain.com"

	secretName, tlsConfig, _ := ingress.CreateTLSSecret(ctx, t, clients, []string{publicHostname})
	publicIng, client, _ := ingress.CreateIngressReadyWithTLS(ctx, t, clients, v1alpha1.IngressSpec{
		Rules: []v1alpha1.IngressRule{{
			Hosts:      []string{publicHostname},
			Visibility: v1alpha1.IngressVisibilityExternalIP,
			HTTP: &v1alpha1.HTTPIngressRuleValue{
				Paths: []v1alpha1.HTTPIngressPath{{
					RewriteHost: privateHostName,
					Splits: []v1alpha1.IngressBackendSplit{{
						AppendHeaders: map[string]string{
							"K-Original-Host": publicHostname,
						},
						IngressBackend: v1alpha1.IngressBackend{
							ServiceName:      privateServiceName,
							ServiceNamespace: test.ServingNamespace,
							ServicePort:      intstr.FromInt(80),
						},
					}},
				}},
			},
		}},
		TLS: []v1alpha1.IngressTLS{{
			Hosts:           []string{publicHostname},
			SecretName:      secretName,
			SecretNamespace: test.ServingNamespace,
		}},
	}, tlsConfig)

	client.Transport = &http.Transport{
		DialContext:     createDialContextProxyProtocol(ctx, t, publicIng, clients),
		TLSClientConfig: tlsConfig,
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+publicHostname, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	assert.Check(t, err == nil)
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

// createDialContextProxyProtocol - create a custom dialer
// it's the same function defined in knative test ingress package, adding to it proxy protocol headers
func createDialContextProxyProtocol(ctx context.Context, t *testing.T, ing *v1alpha1.Ingress, clients *test.Clients) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	if ing.Status.PublicLoadBalancer == nil || len(ing.Status.PublicLoadBalancer.Ingress) < 1 {
		t.Fatal("Ingress does not have a public load balancer assigned.")
	}

	// Create a proxy protocol header
	headerProxyProto := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr: &net.TCPAddr{
			IP:   net.ParseIP("10.1.1.1"),
			Port: 1000,
		},
		DestinationAddr: &net.TCPAddr{
			IP:   net.ParseIP("20.2.2.2"),
			Port: 2000,
		},
	}

	// TODO(mattmoor): I'm open to tricks that would let us cleanly test multiple
	// public load balancers or LBs with multiple ingresses (below), but want to
	// keep our simple tests simple, thus the [0]s...

	// We expect an ingress LB with the form foo.bar.svc.cluster.local (though
	// we aren't strictly sensitive to the suffix, this is just illustrative.
	internalDomain := ing.Status.PublicLoadBalancer.Ingress[0].DomainInternal
	parts := strings.SplitN(internalDomain, ".", 3)
	if len(parts) < 3 {
		t.Fatal("Too few parts in internal domain:", internalDomain)
	}
	name, namespace := parts[0], parts[1]

	var svc *corev1.Service
	err := reconciler.RetryTestErrors(func(_ int) (err error) {
		svc, err = clients.KubeClient.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
	if err != nil {
		t.Fatalf("Unable to retrieve Kubernetes service %s/%s: %v", namespace, name, err)
	}

	dialBackoff := wait.Backoff{
		Duration: 50 * time.Millisecond,
		Factor:   1.4,
		Jitter:   0.1, // At most 10% jitter.
		Steps:    100,
		Cap:      10 * time.Second,
	}

	dial := network.NewBackoffDialer(dialBackoff)
	if pkgTest.Flags.IngressEndpoint != "" {
		t.Logf("ingressendpoint: %q", pkgTest.Flags.IngressEndpoint)

		// If we're using a manual --ingressendpoint then don't require
		// "type: LoadBalancer", which may not play nice with KinD
		return func(ctx context.Context, _ string, address string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			for _, sp := range svc.Spec.Ports {
				if strconv.Itoa(int(sp.Port)) == port {
					conn, err := dial(ctx, "tcp", fmt.Sprintf("%s:%d", pkgTest.Flags.IngressEndpoint, sp.NodePort))
					if err != nil {
						return nil, err
					}
					_, err = headerProxyProto.WriteTo(conn)
					return conn, err
				}
			}
			return nil, fmt.Errorf("service doesn't contain a matching port: %s", port)
		}
	} else if len(svc.Status.LoadBalancer.Ingress) >= 1 {
		ingress := svc.Status.LoadBalancer.Ingress[0]
		return func(ctx context.Context, _ string, address string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if ingress.IP != "" {
				conn, err := dial(ctx, "tcp", ingress.IP+":"+port)
				if err != nil {
					return nil, err
				}
				_, err = headerProxyProto.WriteTo(conn)
				return conn, err
			}
			if ingress.Hostname != "" {
				conn, err := dial(ctx, "tcp", ingress.Hostname+":"+port)
				if err != nil {
					return nil, err
				}
				_, err = headerProxyProto.WriteTo(conn)
				return conn, err
			}
			return nil, errors.New("service ingress does not contain dialing information")
		}
	}

	t.Fatal("Service does not have a supported shape (not type LoadBalancer? missing --ingressendpoint?).")
	return nil // Unreachable
}

func createExternalNameService(ctx context.Context, t *testing.T, clients *test.Clients, target, gatewayDomain string) context.CancelFunc {
	t.Helper()

	targetName := strings.SplitN(target, ".", 3)
	externalNameSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetName[0],
			Namespace: targetName[1],
		},
		Spec: corev1.ServiceSpec{
			Type:            corev1.ServiceTypeExternalName,
			ExternalName:    gatewayDomain,
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports: []corev1.ServicePort{{
				Name:       networking.ServicePortNameH2C,
				Port:       int32(80),
				TargetPort: intstr.FromInt(80),
			}},
		},
	}

	return createService(ctx, t, clients, externalNameSvc)
}

// createService is a helper for creating the service resource.
func createService(ctx context.Context, t *testing.T, clients *test.Clients, svc *corev1.Service) context.CancelFunc {
	t.Helper()

	svcName := ktypes.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}

	t.Cleanup(func() {
		clients.KubeClient.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{})
	})
	if err := reconciler.RetryTestErrors(func(attempts int) error {
		if attempts > 0 {
			t.Logf("Attempt %d creating service %s", attempts, svc.Name)
		}
		_, err := clients.KubeClient.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			t.Logf("Attempt %d creating service failed with: %v", attempts, err)
		}
		return err
	}); err != nil {
		t.Fatalf("Error creating Service %q: %v", svcName, err)
	}

	return func() {
		err := clients.KubeClient.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{})
		if err != nil {
			t.Errorf("Error cleaning up Service %q: %v", svcName, err)
		}
	}
}
