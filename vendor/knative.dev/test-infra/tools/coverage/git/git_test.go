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
package git

import (
	"fmt"
	"os"
	"path"
	"testing"

	"knative.dev/test-infra/tools/coverage/test"
)

var (
	lingGenFilePath = path.Join(test.CovTargetDir, "ling-gen.go")
	covExclFilePath = path.Join(test.CovTargetDir, "cov-excl.go")
	noAttrFilePath  = path.Join(test.CovTargetDir, "no-attr.go")
)

func TestHasGitAttrLingGenPositive(t *testing.T) {
	fmt.Printf("getenv=*%v*\n", os.Getenv("GOPATH"))
	fmt.Printf("filePath=*%s*\n", lingGenFilePath)

	if !hasGitAttr(gitAttrLinguistGenerated, lingGenFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrLingGenNegative(t *testing.T) {
	if hasGitAttr(gitAttrLinguistGenerated, noAttrFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrCovExcPositive(t *testing.T) {
	if !hasGitAttr(gitAttrCoverageExcluded, covExclFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrCovExcNegative(t *testing.T) {
	if hasGitAttr(gitAttrCoverageExcluded, noAttrFilePath) {
		t.Fail()
	}
}
