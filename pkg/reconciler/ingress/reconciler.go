package ingress

import (
	"context"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"

	"k8s.io/apimachinery/pkg/labels"
	kubeclient "k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	networkingV1Alpha "knative.dev/serving/pkg/client/listers/networking/v1alpha1"
)

// Values for the keys received in the reconciler. When they're not a standard
// "namespace/name".
const (
	FullResync     = "full_resync"
	EndpointChange = "endpoint_change"
)

type ResyncAction int

const (
	ResyncAll ResyncAction = iota
	DeleteIngress
	NoAction
)

type Reconciler struct {
	IngressLister   networkingV1Alpha.IngressLister
	EndpointsLister corev1listers.EndpointsLister
	EnvoyXDSServer  envoy.EnvoyXdsServer
	kubeClient      kubeclient.Interface
	CurrentCaches   *envoy.Caches
}

func (reconciler *Reconciler) Reconcile(ctx context.Context, key string) error {
	action, err := actionNeeded(key, reconciler.IngressLister)

	if err != nil {
		return err
	}

	switch action {
	case ResyncAll:
		err := reconciler.fullReconcile()
		if err != nil {
			return err
		}
	case DeleteIngress:
		ingressNamespace, ingressName, err := cache.SplitMetaNamespaceKey(key)

		if err != nil {
			return err
		}

		reconciler.deleteIngress(ingressName, ingressNamespace)
	}

	return nil
}

// TODO: For now, it always returns full resync unless it detects that the
// ingress has been deleted. This should be optimized in the future.
func actionNeeded(key string, ingressLister networkingV1Alpha.IngressLister) (ResyncAction, error) {
	if key == FullResync || key == EndpointChange {
		return ResyncAll, nil
	}

	ingressNamespace, ingressName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return NoAction, err
	}

	ingressExists, err := knative.Exists(ingressName, ingressNamespace, ingressLister)
	if err != nil {
		return NoAction, err
	}

	if !ingressExists {
		return DeleteIngress, nil
	}

	return ResyncAll, nil
}

func (reconciler *Reconciler) fullReconcile() error {
	ingresses, err := reconciler.IngressLister.List(labels.Everything())
	if err != nil {
		return err
	}

	kourierIngresses := knative.FilterByIngressClass(ingresses)

	caches := reconciler.EnvoyXDSServer.SetSnapshotForIngresses(
		nodeID,
		kourierIngresses,
		reconciler.EndpointsLister,
	)

	reconciler.CurrentCaches = caches

	return nil
}

func (reconciler *Reconciler) deleteIngress(ingressName string, ingressNamespace string) {
	reconciler.CurrentCaches.DeleteIngressInfo(ingressName, ingressNamespace, reconciler.kubeClient)
	reconciler.EnvoyXDSServer.SetSnapshotForCaches(reconciler.CurrentCaches, nodeID)
}
