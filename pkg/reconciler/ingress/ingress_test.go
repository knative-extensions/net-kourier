/*
Copyright 2025 The Knative Authors

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

package ingress

import (
	"context"
	"testing"
	"time"

	xds "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	fakenetworkingclient "knative.dev/networking/pkg/client/injection/client/fake"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	netconfig "knative.dev/networking/pkg/config"
	"knative.dev/networking/pkg/status"
	fakekubeclient "knative.dev/pkg/client/injection/kube/client/fake"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
	"knative.dev/pkg/reconciler"
	rtesting "knative.dev/pkg/reconciler/testing"
	"knative.dev/pkg/tracker"

	"knative.dev/net-kourier/pkg/envoy/server"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/net-kourier/pkg/reconciler/ingress/config"
	krtesting "knative.dev/net-kourier/pkg/reconciler/testing"
)

func TestReconcile(t *testing.T) {
	table := rtesting.TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}, {
		Name: "skip ingress not matching class key",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing("name", "ns",
				withAnnotation(map[string]string{
					networking.IngressClassAnnotationKey: "fake-controller",
				})),
		},
	}, {
		Name: "skip ingress marked for deletion",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing("name", "ns",
				withKourier, func(i *v1alpha1.Ingress) {
					i.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				}),
		},
	}, {
		Name: "first reconcile basic ingress",
		Key:  "ns/name",
		Objects: []runtime.Object{
			ing("name", "ns", withBasicSpec, withKourier),
			&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.InternalServiceName,
					Namespace: config.GatewayNamespace(),
				},
				Subsets: []corev1.EndpointSubset{{
					Addresses: []corev1.EndpointAddress{{IP: "2.2.2.2"}},
				}},
			},
		},
		WantEvents: []string{
			rtesting.Eventf(corev1.EventTypeNormal, "FinalizerUpdate", "Updated %q finalizers", "name"),
		},
		WantPatches: []clientgotesting.PatchActionImpl{{
			Name:  "name",
			Patch: []byte(`{"metadata":{"finalizers":["ingresses.networking.internal.knative.dev"],"resourceVersion":""}}`),
		}},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: ing("name", "ns", withBasicSpec, withKourier, func(i *v1alpha1.Ingress) {
				i.Status.InitializeConditions()
				i.Status.MarkNetworkConfigured()
				i.Status.MarkLoadBalancerNotReady()
			}),
		}},
	}}

	table.Test(t, func(t *testing.T, tr *rtesting.TableRow) (
		controller.Reconciler, rtesting.ActionRecorderList, rtesting.EventList,
	) {
		ls := krtesting.NewListers(tr.Objects)
		ctx := tr.Ctx
		if ctx == nil {
			ctx = context.Background()
		}

		it := generator.NewIngressTranslator(
			func(ns, name string) (*corev1.Secret, error) {
				return ls.GetSecretLister().Secrets(ns).Get(name)
			},
			func(_ string) ([]*corev1.ConfigMap, error) {
				return ls.GetConfigMapLister().List(labels.NewSelector())
			},
			func(ns, name string) (*corev1.Endpoints, error) {
				return ls.GetEndpointsLister().Endpoints(ns).Get(name)
			},
			func(ns, name string) (*corev1.Service, error) {
				return ls.GetK8sServiceLister().Services(ns).Get(name)
			},
			&fakeTracker{},
		)

		logger := logtesting.TestLogger(t)
		ctx = logging.WithLogger(ctx, logger)

		eventRecorder := record.NewFakeRecorder(10)
		ctx = controller.WithEventRecorder(ctx, eventRecorder)

		ctx, client := fakenetworkingclient.With(ctx, ls.GetNetworkingObjects()...)
		ctx, kubeclient := fakekubeclient.With(ctx, ls.GetKubeObjects()...)

		// This is needed by the Configuration controller tests, which
		// use GenerateName to produce Revisions.
		rtesting.PrependGenerateNameReactor(&client.Fake)
		rtesting.PrependGenerateNameReactor(&kubeclient.Fake)

		ctx = config.ToContext(ctx, config.FromContextOrDefaults(ctx))

		c, _ := generator.NewCaches(ctx, kubeclient)

		r := &Reconciler{
			xdsServer:         server.NewXdsServer(18000, &xds.CallbackFuncs{}),
			caches:            c,
			ingressTranslator: &it,
			extAuthz:          false,
			resyncConflicts:   func() {},
			statusManager: status.NewProber(
				nil, NewProbeTargetLister(logging.FromContext(ctx), ls.GetEndpointsLister()), nil,
			),
		}

		rr := ingressreconciler.NewReconciler(ctx,
			logging.FromContext(ctx), fakenetworkingclient.Get(ctx),
			ls.GetIngressLister(), controller.GetEventRecorder(ctx), r, config.KourierIngressClassName,
			controller.Options{
				ConfigStore: &testConfigStore{
					config: ReconcilerTestConfig(),
				},
			},
		)

		// Update the context with the stuff we decorated it with.
		tr.Ctx = ctx //nolint:fatcontext

		if la, ok := rr.(reconciler.LeaderAware); ok {
			la.Promote(reconciler.UniversalBucket(), func(reconciler.Bucket, types.NamespacedName) {})
		}

		actionRecorderList := rtesting.ActionRecorderList{client, kubeclient}
		eventList := rtesting.EventList{Recorder: eventRecorder}

		return rr, actionRecorderList, eventList

	})

}

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

var _ reconciler.ConfigStore = (*testConfigStore)(nil)

func ReconcilerTestConfig() *config.Config {
	return &config.Config{
		Kourier: &config.Kourier{},
		Network: &netconfig.Config{
			ExternalDomainTLS: false,
		},
	}
}

type fakeTracker struct{}

func (t *fakeTracker) Track(_ corev1.ObjectReference, _ interface{}) error     { return nil }
func (t *fakeTracker) TrackReference(_ tracker.Reference, _ interface{}) error { return nil }
func (t *fakeTracker) OnChanged(_ interface{})                                 {}
func (t *fakeTracker) GetObservers(_ interface{}) []types.NamespacedName {
	return []types.NamespacedName{}
}
func (t *fakeTracker) OnDeletedObserver(_ interface{}) {}
