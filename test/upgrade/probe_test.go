//go:build probe
// +build probe

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

package upgrade

import (
	"context"
	"errors"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"syscall"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/test"
	"knative.dev/networking/test/conformance/ingress"
)

const probePipe = "/tmp/prober-signal"

var successFraction = flag.Float64("probe.success_fraction", 1.0, "Fraction of probes required to pass during upgrade.")

func TestProbe(t *testing.T) {
	t.Parallel()
	// We run the prober as a golang test because it fits in nicely with
	// the rest of our integration tests, and AssertProberDefault needs
	// a *testing.T. Unfortunately, "go test" intercepts signals, so we
	// can't coordinate with the test by just sending e.g. SIGCONT, so we
	// create a named pipe and wait for the upgrade script to write to it
	// to signal that we should stop probing.
	createPipe(t, probePipe)

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

	proberCancel := checkOK(ctx, t, "http://"+name+".example.com", client)
	defer proberCancel()

	// e2e-upgrade-test.sh will close this pipe to signal the upgrade is
	// over, at which point we will finish the test and check the prober.
	ioutil.ReadFile(probePipe)
}

func checkOK(ctx context.Context, t *testing.T, url string, client *http.Client) context.CancelFunc {
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	// Launch the prober
	go func() {
		defer close(doneCh)

		var successes, failures int
		for {
			// Each iteration check for cancellation.
			select {
			case <-stopCh:
				t.Logf("Finished probing, successes: %d, failures: %d", successes, failures)
				return
			default:
			}
			// Scope the defer below to avoid leaking until the test completes.
			func() {
				ri := ingress.RuntimeRequest(ctx, t, client, url)
				if ri != nil {
					successes++
				} else {
					failures++
				}
			}()
		}
	}()

	// Return a cancel function that stops the prober and then waits for it to complete.
	return func() {
		close(stopCh)
		<-doneCh
	}
}

// createPipe create a named pipe. It fails the test if any error except
// already exist happens.
func createPipe(t *testing.T, name string) {
	if err := syscall.Mkfifo(name, 0666); err != nil {
		if !errors.Is(err, os.ErrExist) {
			t.Fatal("Failed to create pipe:", err)
		}
	}

	test.EnsureCleanup(t, func() {
		os.Remove(name)
	})
}
