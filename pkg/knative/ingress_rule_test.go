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

package knative

import (
	"sort"
	"testing"

	"gotest.tools/assert"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
)

var testRule = v1alpha1.IngressRule{
	Hosts: []string{
		"helloworld-go.default.svc.cluster.local",
		"helloworld-go.default.example.com",
	},
}

func TestExternalDomains(t *testing.T) {
	externalDomains := ExternalDomains(testRule, "cluster.local")

	expected := []string{
		"helloworld-go.default.example.com",
		"helloworld-go.default.example.com:*",
	}
	sort.Strings(externalDomains)
	sort.Strings(expected)
	assert.DeepEqual(t, externalDomains, expected)
}

func TestInternalDomains(t *testing.T) {
	internalDomains := InternalDomains(testRule, "cluster.local")

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

	assert.Equal(t, RuleIsExternal(externalRule, ""), true)
	assert.Equal(t, RuleIsExternal(internalRule, ""), false)
}

func TestRuleIsExternalWithIngressVisibility(t *testing.T) {
	ruleWithoutVisibility := v1alpha1.IngressRule{Visibility: ""}

	assert.Equal(
		t, RuleIsExternal(ruleWithoutVisibility, v1alpha1.IngressVisibilityClusterLocal), false,
	)
	assert.Equal(
		t, RuleIsExternal(ruleWithoutVisibility, v1alpha1.IngressVisibilityExternalIP), true,
	)
}

func TestRuleIsExternalWithoutVisibility(t *testing.T) {
	ruleWithoutVisibility := v1alpha1.IngressRule{Visibility: ""}

	// Knative defaults to external, so it should return true
	assert.Equal(t, RuleIsExternal(ruleWithoutVisibility, ""), true)
}
