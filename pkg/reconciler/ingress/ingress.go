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

	"k8s.io/apimachinery/pkg/types"
	"knative.dev/net-kourier/pkg/config"
	envoy "knative.dev/net-kourier/pkg/envoy/server"
	"knative.dev/net-kourier/pkg/generator"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	"knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
)

const conflictReason = "DomainConflict"

type Reconciler struct {
	xdsServer         *envoy.XdsServer
	caches            *generator.Caches
	statusManager     *status.Prober
	ingressTranslator *generator.IngressTranslator
	extAuthz          bool

	// resyncConflicts triggers a filtered global resync to reenqueue all ingresses in
	// a "Conflict" state.
	resyncConflicts func()
}

var _ ingress.Interface = (*Reconciler)(nil)
var _ ingress.ReadOnlyInterface = (*Reconciler)(nil)
var _ ingress.Finalizer = (*Reconciler)(nil)
var _ reconciler.OnDeletionInterface = (*Reconciler)(nil)

func (r *Reconciler) ReconcileKind(ctx context.Context, ing *v1alpha1.Ingress) reconciler.Event {
	ing.SetDefaults(ctx)
	before := ing.DeepCopy()

	if err := r.updateIngress(ctx, ing); errors.Is(err, generator.ErrDomainConflict) {
		// If we had an error due to a duplicated domain, we must mark the ingress as failed with a
		// custom status. We don't want to return an error in this case as we want to update its status.
		logging.FromContext(ctx).Info(err.Error())
		ing.Status.MarkLoadBalancerFailed(conflictReason, "Ingress rejected: "+err.Error())
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to update ingress: %w", err)
	}

	ing.Status.MarkNetworkConfigured()
	if !ing.IsReady() || !isExpectedLoadBalancer(ing) {
		ready, err := r.statusManager.IsReady(ctx, before)
		if err != nil {
			return fmt.Errorf("failed to probe Ingress: %w", err)
		}
		if ready {
			external, internal := config.ServiceHostnames()
			ing.Status.MarkLoadBalancerReady(
				[]v1alpha1.LoadBalancerIngressStatus{{DomainInternal: external}},
				[]v1alpha1.LoadBalancerIngressStatus{{DomainInternal: internal}},
			)
		} else {
			ing.Status.MarkLoadBalancerNotReady()
		}
	}

	return nil
}

// isExpectedLoadBalancer verifies if expected Loadbalancer is set in status field.
func isExpectedLoadBalancer(ing *v1alpha1.Ingress) bool {
	external, internal := config.ServiceHostnames()
	if ing.Status.PublicLoadBalancer == nil || len(ing.Status.PublicLoadBalancer.Ingress) < 1 ||
		ing.Status.PublicLoadBalancer.Ingress[0].DomainInternal != external {
		return false
	}
	if ing.Status.PrivateLoadBalancer == nil || len(ing.Status.PrivateLoadBalancer.Ingress) < 1 ||
		ing.Status.PrivateLoadBalancer.Ingress[0].DomainInternal != internal {
		return false
	}
	return true
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

func (r *Reconciler) ObserveDeletion(ctx context.Context, key types.NamespacedName) error {
	logger := logging.FromContext(ctx)
	logger.Infof("Ingress deleted, updating config")

	r.statusManager.CancelIngressProbingByKey(key)

	if err := r.caches.DeleteIngressInfo(ctx, key.Name, key.Namespace); err != nil {
		return err
	}

	if err := r.updateEnvoyConfig(ctx); err != nil {
		return fmt.Errorf("failed updating envoy config: %w", err)
	}

	r.resyncConflicts()
	return nil
}

func (r *Reconciler) FinalizeKind(ctx context.Context, ing *v1alpha1.Ingress) reconciler.Event {
	// Keeping this for now to keep finalizer based logic intact.
	return nil
}

func (r *Reconciler) updateIngress(ctx context.Context, ingress *v1alpha1.Ingress) error {
	logger := logging.FromContext(ctx)
	logger.Infof("Updating Ingress")

	if err := generator.UpdateInfoForIngress(
		ctx, r.caches, ingress, r.ingressTranslator, r.extAuthz); err != nil {
		return err
	}

	return r.updateEnvoyConfig(ctx)
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
