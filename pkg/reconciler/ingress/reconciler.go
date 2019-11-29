package ingress

import (
	"context"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"

	"knative.dev/serving/pkg/apis/networking/v1alpha1"

	"knative.dev/pkg/network"

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
	UpdateIngress
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

	ingressNamespace, ingressName, err := cache.SplitMetaNamespaceKey(key)

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
		reconciler.deleteIngress(ingressName, ingressNamespace)
	case UpdateIngress:
		err = reconciler.updateIngress(ingressName, ingressNamespace)

		if err != nil {
			return err
		}
	}

	return nil
}

// TODO: For now, it returns FullResync when an endpoint has been changed. That
// can be optimized.
func actionNeeded(key string, ingressLister networkingV1Alpha.IngressLister) (ResyncAction, error) {
	if key == FullResync || key == EndpointChange {
		return ResyncAll, nil
	}

	// At this point we know that the event has been caused by an ingress.

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

	return UpdateIngress, nil
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

func (reconciler *Reconciler) updateIngress(ingressName string, ingressNamespace string) error {
	ingress, err := reconciler.IngressLister.Ingresses(ingressNamespace).Get(ingressName)

	if err != nil {
		return err
	}

	envoy.UpdateInfoForIngress(
		reconciler.CurrentCaches,
		ingress,
		reconciler.kubeClient,
		reconciler.EndpointsLister,
		network.GetClusterDomainName(),
	)

	reconciler.EnvoyXDSServer.SetSnapshotForCaches(reconciler.CurrentCaches, nodeID)

	reconciler.EnvoyXDSServer.MarkIngressesReady(
		[]*v1alpha1.Ingress{ingress},
		reconciler.CurrentCaches.SnapshotVersion(),
	)

	return nil
}
