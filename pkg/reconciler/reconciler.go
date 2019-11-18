package reconciler

import (
	"context"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"

	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/serving/pkg/client/listers/networking/v1alpha1"
)

type Reconciler struct {
	IngressLister   v1alpha1.IngressLister
	EndpointsLister corev1listers.EndpointsLister
	EnvoyXDSServer  envoy.EnvoyXdsServer
}

func (reconciler *Reconciler) Reconcile(ctx context.Context, key string) error {
	ingressAccessors, err := reconciler.IngressLister.List(labels.Everything())
	if err != nil {
		return err
	}

	kourierIngresses := knative.FilterByIngressClass(ingressAccessors)

	reconciler.EnvoyXDSServer.SetSnapshotForIngresses(nodeID, kourierIngresses)

	return nil
}
