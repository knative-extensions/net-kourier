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
package line

import (
	"testing"

	"knative.dev/test-infra/tools/coverage/artifacts/artsTest"
	"knative.dev/test-infra/tools/coverage/test"
)

func TestCreateLineCovFile(t *testing.T) {
	arts := artsTest.LocalArtsForTest("TestCreateLineCovFile")
	test.LinkInputArts(arts.Directory(), "key-cov-profile.txt")

	err := CreateLineCovFile(arts)
	if err != nil {
		t.Fatalf("CreateLineCovFile(arts=%v) failed, err=%v", arts, err)
	}
	test.DeleteDir(arts.Directory())
}

func TestCreateLineCovFileFailure(t *testing.T) {
	arts := artsTest.LocalArtsForTest_KeyfileNotExist("TestCreateLineCovFileFailure")
	if CreateLineCovFile(arts) == nil {
		t.Fatalf("CreateLineCovFile(arts=%v) should fail, but not", arts)
	}
}
