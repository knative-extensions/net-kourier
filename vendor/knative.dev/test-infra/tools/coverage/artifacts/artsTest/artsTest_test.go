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

package artsTest

import (
	"testing"

	"knative.dev/test-infra/tools/coverage/test"
)

func TestLocalInputArts(t *testing.T) {
	arts := LocalInputArtsForTest()
	if test.FileOrDirExists(arts.Directory()) == false {
		t.Fatalf("FileOrDirExists(arts.Directory()) == false\n")
	}
	if test.FileOrDirExists(arts.ProfilePath()) == false {
		t.Fatalf("Profile File not exist\n")
	}
	if test.FileOrDirExists(arts.KeyProfilePath()) == false {
		t.Fatalf("Key Profile File not exist\n")
	}
	if test.FileOrDirExists(arts.CovStdoutPath()) == false {
		t.Fatalf("FileOrDirExists(arts.CovStdoutPath()) == false\n")
	}
}
