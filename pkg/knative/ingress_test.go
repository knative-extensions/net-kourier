package knative

import (
	"testing"

	"gotest.tools/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/apis/networking"
	networkingv1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
)

func TestFilterByIngressClass(t *testing.T) {
	kourierIngress := networkingv1alpha1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				networking.IngressClassAnnotationKey: "kourier.ingress.networking.knative.dev",
			},
		},
	}

	unknownIngress := networkingv1alpha1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				networking.IngressClassAnnotationKey: "unknown",
			},
		},
	}

	tests := map[string]struct {
		inputIngresses []*networkingv1alpha1.Ingress
		want           []*networkingv1alpha1.Ingress
	}{
		"no input ingresses": {
			inputIngresses: []*networkingv1alpha1.Ingress{},
			want:           []*networkingv1alpha1.Ingress{},
		},
		"some Kourier ingresses": {
			inputIngresses: []*networkingv1alpha1.Ingress{&kourierIngress, &unknownIngress},
			want:           []*networkingv1alpha1.Ingress{&kourierIngress},
		},
		"no Kourier ingresses": {
			inputIngresses: []*networkingv1alpha1.Ingress{&unknownIngress},
			want:           []*networkingv1alpha1.Ingress{},
		},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			got := FilterByIngressClass(data.inputIngresses)
			assert.DeepEqual(t, data.want, got)
		})
	}
}

func TestRouteName(t *testing.T) {
	routeName := "some_route_name"
	ingress := networkingv1alpha1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{
				"serving.knative.dev/route": routeName,
			},
		},
	}

	assert.Equal(t, routeName, RouteName(&ingress))
}

func TestRouteNamespace(t *testing.T) {
	routeNamespace := "some_route_namespace"
	ingress := networkingv1alpha1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{
				"serving.knative.dev/routeNamespace": routeNamespace,
			},
		},
	}

	assert.Equal(t, routeNamespace, RouteNamespace(&ingress))
}
