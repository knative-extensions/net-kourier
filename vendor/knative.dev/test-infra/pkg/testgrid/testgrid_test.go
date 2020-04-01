/*
Copyright 2019 The Knative Authors

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

package testgrid

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"knative.dev/test-infra/pkg/junit"
	"knative.dev/test-infra/pkg/prow"
)

const (
	filename = "junit_test.xml"
	name     = "test"
)

func checkFileText(resultFile, expected string, t *testing.T) {
	d, err := ioutil.ReadFile(resultFile)
	s := string(d)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	if s != expected {
		t.Fatalf("Got:\n%s, Want:\n %s", s, expected)
	}
}

func TestXMLOutput(t *testing.T) {
	resultFile := path.Join(prow.GetLocalArtifactsDir(), filename)
	defer os.Remove(resultFile)

	// Create a test suites
	tc := []junit.TestCase{}
	want := `<testsuites>
  <testsuite name="test" time="0" failures="0" tests="0">
    <properties></properties>
  </testsuite>
</testsuites>
`

	// Create a test file
	if err := CreateXMLOutput(tc, name); err != nil {
		t.Fatalf("Error when creating xml output file: %v", err)
	}
	checkFileText(resultFile, want, t)
}

func TestConfigPath(t *testing.T) {
	if _, err := NewConfig(); err != nil {
		t.Fatalf("Testing default config file, want: no err, got: %v", err)
	}
}

func TestTabName(t *testing.T) {
	ac, _ := NewConfig()
	for tgName, URL := range jobNameTestgridURLMap {
		if got, _ := ac.GetTabRelURL(tgName); got != URL {
			t.Fatalf("Testing testgroup/tab mapping for '%s', want: '%s', got: '%s'", tgName, URL, got)
		}
	}
}
