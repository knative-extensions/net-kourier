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
package artifacts

import (
	"bufio"
	"fmt"
	"log"
	"testing"

	"knative.dev/test-infra/tools/coverage/logUtil"
	"knative.dev/test-infra/tools/coverage/test"
)

// generates coverage profile by running go test on target package
func TestProfiling(t *testing.T) {
	logUtil.LogFatalf = log.Fatalf

	arts := LocalArtifacts{
		Artifacts: *New(
			test.NewArtsDir(t.Name()),
			"cov-profile.txt",
			"key-cov-profile.txt",
			"stdout.txt"),
	}
	arts.ProduceProfileFile(fmt.Sprintf("../%s/subPkg1/ "+
		"../%s/subPkg2/", test.CovTargetRootRel, test.CovTargetRootRel))

	t.Logf("Verifying profile file...\n")
	expectedFirstLine := "mode: count"
	expectedLine := "knative.dev/test-infra/tools/coverage/testTarget/subPkg1/common.go:19.19,21.2 0 2"

	scanner := bufio.NewScanner(arts.ProfileReader())
	scanner.Scan()
	if scanner.Text() != expectedFirstLine {
		t.Fatalf("File should start with the line '%s';\nit actually starts with '%s'", expectedFirstLine, scanner.Text())
	}

	for scanner.Scan() {
		if scanner.Text() == expectedLine {
			t.Logf("found expected line, test succeeded")
			return
		}
	}

	t.Fatalf("line not found '%s'", expectedLine)
}
