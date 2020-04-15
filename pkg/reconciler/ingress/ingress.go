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

package ingress

import (
	"context"
	"fmt"
	"reflect"

	"knative.dev/net-kourier/pkg/config"
	"knative.dev/net-kourier/pkg/errors"

	"knative.dev/serving/pkg/network/status"

	"knative.dev/net-kourier/pkg/envoy"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/net-kourier/pkg/knative"

	"go.uber.org/zap"

	"knative.dev/serving/pkg/apis/networking/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	knativeclient "knative.dev/serving/pkg/client/clientset/versioned"
	nv1alpha1lister "knative.dev/serving/pkg/client/listers/networking/v1alpha1"
)

type Reconciler struct {
	IngressLister     nv1alpha1lister.IngressLister
	EnvoyXDSServer    *envoy.XdsServer
	kubeClient        kubeclient.Interface
	knativeClient     knativeclient.Interface
	CurrentCaches     *generator.Caches
	statusManager     *status.Prober
	ingressTranslator *generator.IngressTranslator
	ExtAuthz          bool
	logger            *zap.SugaredLogger
}

func (reconciler *Reconciler) Reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	reconciler.logger.Infof("Got reconcile request for %s namespace: %s", name, namespace)

	original, err := reconciler.IngressLister.Ingresses(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return reconciler.deleteIngress(namespace, name)
	} else if err != nil {
		return err
	}

	ingress := original.DeepCopy()
	ingress.SetDefaults(ctx)
	ingress.Status.InitializeConditions()
	// This is the generation we are going to handle during this reconciliation loop.
	ingress.Status.ObservedGeneration = ingress.GetGeneration()

	err = reconciler.updateIngress(ingress)
	if err != nil && err == errors.ErrDomainConflict {
		// If we had an error due to a duplicated domain, we must mark the ingress as failed with a
		// custom status. We don't want to return an error in this case as we want to update its status.
		reconciler.logger.Errorw(err.Error(), ingress.Name, ingress.Namespace)
		ingress.Status.MarkLoadBalancerFailed(config.DuplicatedDomainReason, config.DuplicatedDomainMessage)
	} else if err != nil {
		return fmt.Errorf("failed to update ingress: %w", err)
	}

	return reconciler.updateStatus(original, ingress)
}

func (reconciler *Reconciler) deleteIngress(namespace, name string) error {
	reconciler.logger.Infof("Deleting Ingress %s namespace: %s", name, namespace)
	ingress := reconciler.CurrentCaches.GetIngress(name, namespace)

	// We need to check for ingress not being nil, because we can receive an event from an already
	// removed ingress, like for example, when the endpoints object for that ingress is updated/removed.
	if ingress != nil {
		reconciler.statusManager.CancelIngressProbing(ingress)
	}

	err := reconciler.CurrentCaches.DeleteIngressInfo(name, namespace, reconciler.kubeClient)
	if err != nil {
		return err
	}

	snapshot, err := reconciler.CurrentCaches.ToEnvoySnapshot()
	if err != nil {
		return err
	}

	return reconciler.EnvoyXDSServer.SetSnapshot(&snapshot, nodeID)
}

func (reconciler *Reconciler) updateIngress(ingress *v1alpha1.Ingress) error {
	reconciler.logger.Infof("Updating Ingress %s namespace: %s", ingress.Name, ingress.Namespace)

	// We create a copy of the ingress object to get the original one, without the modifications (we add some headers to the object)
	// so IsReady can generate a clean Hash from ingress that will match.
	var notModifiedIngress v1alpha1.Ingress
	ingress.DeepCopyInto(&notModifiedIngress)

	err := generator.UpdateInfoForIngress(
		reconciler.CurrentCaches, ingress, reconciler.kubeClient, reconciler.ingressTranslator, reconciler.logger, reconciler.ExtAuthz,
	)
	if err != nil {
		return err
	}

	snapshot, err := reconciler.CurrentCaches.ToEnvoySnapshot()
	if err != nil {
		return err
	}

	err = reconciler.EnvoyXDSServer.SetSnapshot(&snapshot, nodeID)
	if err != nil {
		return err
	}

	ready, err := reconciler.statusManager.IsReady(context.TODO(), &notModifiedIngress)
	if err != nil {
		return err
	}

	if ready {
		knative.MarkIngressReady(ingress)
	}
	return nil
}

func (reconciler *Reconciler) updateStatus(existing *v1alpha1.Ingress, desired *v1alpha1.Ingress) error {
	// If there's nothing to update, just return.
	if reflect.DeepEqual(existing.Status, desired.Status) {
		return nil
	}

	existing = existing.DeepCopy()
	existing.Status = desired.Status
	_, err := reconciler.knativeClient.NetworkingV1alpha1().Ingresses(existing.Namespace).UpdateStatus(existing)
	return err
}
