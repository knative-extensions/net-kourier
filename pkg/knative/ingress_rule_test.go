package knative

import (
	"sort"
	"testing"

	"gotest.tools/assert"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
)

var testRule = v1alpha1.IngressRule{
	Hosts: []string{
		"helloworld-go.default.svc.cluster.local",
		"helloworld-go.default.example.com",
	},
}

func TestExternalDomains(t *testing.T) {
	externalDomains := ExternalDomains(&testRule, "cluster.local")

	expected := []string{
		"helloworld-go.default.example.com",
		"helloworld-go.default.example.com:*",
	}
	sort.Strings(externalDomains)
	sort.Strings(expected)
	assert.DeepEqual(t, externalDomains, expected)
}

func TestInternalDomains(t *testing.T) {
	internalDomains := InternalDomains(&testRule, "cluster.local")

	expected := []string{
		"helloworld-go.default",
		"helloworld-go.default:*",
		"helloworld-go.default.svc",
		"helloworld-go.default.svc:*",
		"helloworld-go.default.svc.cluster.local",
		"helloworld-go.default.svc.cluster.local:*",
	}
	sort.Strings(internalDomains)
	sort.Strings(expected)
	assert.DeepEqual(t, internalDomains, expected)
}

func TestRuleIsExternalWithVisibility(t *testing.T) {
	externalRule := v1alpha1.IngressRule{
		Visibility: v1alpha1.IngressVisibilityExternalIP,
	}
	internalRule := v1alpha1.IngressRule{
		Visibility: v1alpha1.IngressVisibilityClusterLocal,
	}

	assert.Equal(t, RuleIsExternal(&externalRule, ""), true)
	assert.Equal(t, RuleIsExternal(&internalRule, ""), false)
}

func TestRuleIsExternalWithIngressVisibility(t *testing.T) {
	ruleWithoutVisibility := v1alpha1.IngressRule{Visibility: ""}

	assert.Equal(
		t, RuleIsExternal(&ruleWithoutVisibility, v1alpha1.IngressVisibilityClusterLocal), false,
	)
	assert.Equal(
		t, RuleIsExternal(&ruleWithoutVisibility, v1alpha1.IngressVisibilityExternalIP), true,
	)
}

func TestRuleIsExternalWithoutVisibility(t *testing.T) {
	ruleWithoutVisibility := v1alpha1.IngressRule{Visibility: ""}

	// Knative defaults to external, so it should return true
	assert.Equal(t, RuleIsExternal(&ruleWithoutVisibility, ""), true)
}
