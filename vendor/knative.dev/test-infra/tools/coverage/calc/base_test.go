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
package calc

import (
	"testing"

	"knative.dev/test-infra/tools/coverage/test"
)

func testCoverage() (c *Coverage) {
	return &Coverage{name: "fake-coverage", nCoveredStmts: 200, nAllStmts: 300, lineCovLink: "fake-line-cov-url"}
}

func TestCoverageRatio(t *testing.T) {
	c := testCoverage()
	actualRatio, _ := c.Ratio()
	if actualRatio != float32(c.nCoveredStmts)/float32(c.nAllStmts) {
		t.Fatalf("actualRatio != float32(c.nCoveredStmts) / float32(c.nAllStmts)\n")
	}
}

func TestRatioErr(t *testing.T) {
	c := &Coverage{name: "fake-coverage", nCoveredStmts: 200, nAllStmts: 0, lineCovLink: "fake-line-cov-url"}
	_, err := c.Ratio()
	if err == nil {
		t.Fatalf("fail to return err for 0 denominator")
	}
}

func TestPercentageNA(t *testing.T) {
	c := &Coverage{name: "fake-coverage", nCoveredStmts: 200, nAllStmts: 0, lineCovLink: "fake-line-cov-url"}
	test.AssertEqual(t, "N/A", c.Percentage())
}

func TestSort(t *testing.T) {
	cs := []Coverage{
		*newCoverage("pear"),
		*newCoverage("apple"),
		*newCoverage("candy"),
	}
	SortCoverages(cs)

	expected := []string{"apple", "candy", "pear"}
	for i, c := range cs {
		test.AssertEqual(t, expected[i], c.Name())
	}
}
