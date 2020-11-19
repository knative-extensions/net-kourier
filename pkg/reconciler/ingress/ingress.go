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
	"errors"
	"fmt"

	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/server"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

type Reconciler struct {
	xdsServer         *envoy.XdsServer
	caches            *generator.Caches
	statusManager     *status.Prober
	ingressTranslator *generator.IngressTranslator
	extAuthz          bool
}

var _ ingress.Interface = (*Reconciler)(nil)
var _ ingress.ReadOnlyInterface = (*Reconciler)(nil)
var _ ingress.Finalizer = (*Reconciler)(nil)
var _ ingress.ReadOnlyFinalizer = (*Reconciler)(nil)

func (r *Reconciler) ReconcileKind(ctx context.Context, ing *v1alpha1.Ingress) reconciler.Event {
	ing.SetDefaults(ctx)
	before := ing.DeepCopy()

	if err := r.updateIngress(ctx, ing); errors.Is(err, generator.ErrDomainConflict) {
		// If we had an error due to a duplicated domain, we must mark the ingress as failed with a
		// custom status. We don't want to return an error in this case as we want to update its status.
		logging.FromContext(ctx).Info(err.Error())
		ing.Status.MarkLoadBalancerFailed("DomainConflict", "Ingress rejected: "+err.Error())
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to update ingress: %w", err)
	}

	if !ing.IsReady() {
		ready, err := r.statusManager.IsReady(context.TODO(), before)
		if err != nil {
			return fmt.Errorf("failed to probe Ingress %s/%s: %w", ing.GetNamespace(), ing.GetName(), err)
		}
		if ready {
			external, internal := config.ServiceHostnames()
			ing.Status.MarkLoadBalancerReady(
				[]v1alpha1.LoadBalancerIngressStatus{{DomainInternal: external}},
				[]v1alpha1.LoadBalancerIngressStatus{{DomainInternal: internal}},
			)
			ing.Status.MarkNetworkConfigured()
		} else {
			ing.Status.MarkLoadBalancerNotReady()
		}
	}

	return nil
}

func (r *Reconciler) ObserveKind(ctx context.Context, ing *v1alpha1.Ingress) reconciler.Event {
	ing.SetDefaults(ctx)

	if err := r.updateIngress(ctx, ing); errors.Is(err, generator.ErrDomainConflict) {
		// If we had an error due to a duplicated domain, just abort.
		logging.FromContext(ctx).Info(err.Error())
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to update ingress: %w", err)
	}

	return nil
}

func (r *Reconciler) FinalizeKind(ctx context.Context, ing *v1alpha1.Ingress) reconciler.Event {
	return r.ObserveFinalizeKind(ctx, ing)
}
func (r *Reconciler) ObserveFinalizeKind(ctx context.Context, ing *v1alpha1.Ingress) reconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof("Deleting Ingress %s/%s", ing.Name, ing.Namespace)

	r.statusManager.CancelIngressProbing(ing)

	if err := r.caches.DeleteIngressInfo(ctx, ing.Name, ing.Namespace); err != nil {
		return err
	}

	return r.updateEnvoyConfig(ctx)
}

func (r *Reconciler) updateIngress(ctx context.Context, ingress *v1alpha1.Ingress) error {
	logger := logging.FromContext(ctx)
	logger.Infof("Updating Ingress %s namespace: %s", ingress.Name, ingress.Namespace)

	if err := generator.UpdateInfoForIngress(
		ctx, r.caches, ingress, r.ingressTranslator, r.extAuthz); err != nil {
		return err
	}

	if err := r.updateEnvoyConfig(ctx); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) updateEnvoyConfig(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	logger.Debugf("Preparing Envoy Snapshot")
	newSnapshot, err := r.caches.ToEnvoySnapshot(ctx)
	if err != nil {
		return err
	}

	return r.xdsServer.SetSnapshot(nodeID, newSnapshot)
}
