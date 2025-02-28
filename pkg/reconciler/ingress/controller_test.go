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
	"testing"

	_ "knative.dev/networking/pkg/client/injection/client/fake"
	_ "knative.dev/networking/pkg/client/injection/informers/networking/v1alpha1/ingress/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/core/v1/pod/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/core/v1/secret/filtered/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/core/v1/service/fake"
	_ "knative.dev/pkg/client/injection/kube/informers/factory/filtered/fake"
	_ "knative.dev/pkg/injection/clients/namespacedkube/informers/core/v1/configmap/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	networkcfg "knative.dev/networking/pkg/config"
	kubeclient "knative.dev/pkg/client/injection/kube/client/fake"
	"knative.dev/pkg/configmap"
	rtesting "knative.dev/pkg/reconciler/testing"
	"knative.dev/pkg/system"

	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/reconciler/informerfiltering"
)

func TestNew(t *testing.T) {
	ctx, _ := rtesting.SetupFakeContext(t,
		informerfiltering.GetContextWithFilteringLabelSelector,
	)
	_, _ = kubeclient.Get(ctx).CoreV1().ConfigMaps(system.Namespace()).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      config.ConfigName,
		},
	}, metav1.CreateOptions{})
	_, _ = kubeclient.Get(ctx).CoreV1().ConfigMaps(system.Namespace()).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      networkcfg.ConfigMapName,
		},
	}, metav1.CreateOptions{})

	c := NewController(ctx, configmap.NewStaticWatcher(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      config.ConfigName,
		},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      networkcfg.ConfigMapName,
		},
	}))

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}
