/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package githubUtil

import (
	"path"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"knative.dev/test-infra/tools/coverage/githubUtil/githubFakes"
	"knative.dev/test-infra/tools/coverage/test"
)

func TestGetConcernedFiles(t *testing.T) {
	data := githubFakes.FakeRepoData()
	actualConcernMap := GetConcernedFiles(data, test.ProjDir())
	t.Logf("concerned files for PR %v:%v", data.Pr, actualConcernMap)

	expectedConcerns := sets.String{}
	for _, fileName := range []string{
		"common.go",
		"onlySrcChange.go",
		"onlyTestChange.go",
		"newlyAddedFile.go",
	} {
		expectedConcerns.Insert(path.Join(test.CovTargetDir, fileName))
	}

	t.Logf("expected concerns=%v", expectedConcerns.List())
	for fileName, actual := range actualConcernMap {
		if expected := expectedConcerns.Has(fileName); actual != expected {
			t.Fatalf("filename=%s, isConcerned: expected=%v; actual=%v\n", fileName, expected, actual)
		}
	}
}

func TestSourceFilePath(t *testing.T) {
	input := "pkg/fake_test.go"
	actual := sourceFilePath(input)
	expected := "pkg/fake.go"
	if actual != expected {
		t.Fatalf(test.StrFailure(input, actual, expected))
	}

	input = "pkg/fake_2.go"
	actual = sourceFilePath(input)
	expected = "pkg/fake_2.go"
	if actual != expected {
		t.Fatalf(test.StrFailure(input, actual, expected))
	}
}
