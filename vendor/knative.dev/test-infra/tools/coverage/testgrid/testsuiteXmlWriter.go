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
	"fmt"
	"log"
	"os"
	"strconv"

	"knative.dev/test-infra/shared/junit"
	"knative.dev/test-infra/tools/coverage/artifacts"
	"knative.dev/test-infra/tools/coverage/calc"
	"knative.dev/test-infra/tools/coverage/logUtil"
)

// NewTestCase constructs the TestCase struct
func NewTestCase(targetName, coverage string, failure bool) junit.TestCase {
	tc := junit.TestCase{
		ClassName: "go_coverage",
		Name:      targetName,
	}

	// we want to add <failure> tag to only failed tests. Testgrid treats both
	// "<failure> true </failure>" and "<failure> false </failure>" as failure
	if failure {
		f := strconv.FormatBool(failure)
		tc.Failure = &f
	}

	tc.AddProperty("coverage", coverage)

	return tc
}

// toTestsuite populates Testsuite struct with data from CoverageList and actual file
// directories from OS
func toTestsuite(g *calc.CoverageList, dirs []string) *junit.TestSuite {
	g.Summarize()
	covThresInt := g.CovThresInt()
	ts := junit.TestSuite{}

	// Add overall coverage
	ts.AddTestCase(NewTestCase("OVERALL", g.PercentageForTestgrid(), g.IsCoverageLow(covThresInt)))

	fmt.Println("")
	log.Println("Constructing Testsuite Struct for Testgrid")

	// Add coverage for individual files
	for _, cov := range *g.Group() {
		coverage := cov.PercentageForTestgrid()
		if coverage != "" {
			ts.AddTestCase(NewTestCase(cov.Name(), coverage, cov.IsCoverageLow(covThresInt)))
		} else {
			log.Printf("Skipping file %s as it has no coverage data.\n", cov.Name())
		}
	}

	// Add coverage for dirs
	for _, dir := range dirs {
		dirCov := g.Subset(dir)
		coverage := dirCov.PercentageForTestgrid()
		if coverage != "" {
			ts.AddTestCase(NewTestCase(dir, coverage, dirCov.IsCoverageLow(covThresInt)))
		} else {
			log.Printf("Skipping directory %s as it has no files with coverage data.\n", dir)
		}
	}
	log.Println("Finished Constructing Testsuite Struct for Testgrid")
	fmt.Println("")

	return &ts
}

// ProfileToTestsuiteXML uses coverage profile (and it's corresponding stdout) to produce junit xml
// which serves as the input for test coverage testgrid
func ProfileToTestsuiteXML(arts *artifacts.LocalArtifacts, covThres int) {
	groupCov := calc.CovList(
		artifacts.NewProfileReader(arts.ProfileReader()),
		nil,
		nil,
		covThres,
	)
	f, err := os.Create(arts.JunitXmlForTestgridPath())
	if err != nil {
		logUtil.LogFatalf("Cannot create file: %v", err)
	}
	defer f.Close()

	suites := junit.TestSuites{}
	suites.AddTestSuite(toTestsuite(groupCov, groupCov.GetDirs()))
	output, err := suites.ToBytes("", "    ")
	if err != nil {
		logUtil.LogFatalf("error: %v\n", err)
	}

	f.Write(output)
}
