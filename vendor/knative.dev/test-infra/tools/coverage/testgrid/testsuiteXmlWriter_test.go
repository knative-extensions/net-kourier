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
package testgrid

import (
	"testing"

	"knative.dev/test-infra/tools/coverage/artifacts/artsTest"
	"knative.dev/test-infra/tools/coverage/test"
)

const (
	covProfileName = "cov-profile.txt"
	stdoutFileName = "stdout.txt"
)

func TestXMLProduction(t *testing.T) {
	arts := artsTest.LocalArtsForTest("TestXMLProduction")
	test.LinkInputArts(arts.Directory(), covProfileName, stdoutFileName)
	ProfileToTestsuiteXML(arts, 50)
	test.DeleteDir(arts.Directory())
}
