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

// log_parser.go collects failed tests from jobs that triggered the retryer and
// finds flaky tests that are relevant to that failed job.

package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"knative.dev/test-infra/shared/junit"
	"knative.dev/test-infra/shared/prow"
	"knative.dev/test-infra/tools/flaky-test-reporter/jsonreport"

	// TODO: remove this import once "k8s.io/test-infra" import problems are fixed
	// https://github.com/knative/test-infra/test-infra/issues/912
	"knative.dev/test-infra/tools/monitoring/prowapi"
)

var client jsonreport.Client

// InitLogParser configures jsonreport's dependencies.
func InitLogParser(serviceAccount string) error {
	var err error
	client, err = jsonreport.Initialize(serviceAccount)
	return err
}

// JobData contains the message describing a job, a local cache of its failed tests,
// and a cached flaky report it is referencing.
type JobData struct {
	*prowapi.ReportMessage
	failedTests  []string
	flakyReports []jsonreport.Report
}

// IsSupported checks to make sure the message can be processed with the current flaky
// test information
func (jd *JobData) IsSupported() bool {
	prefix := fmt.Sprintf("Job %q(%q) did not fit criteria", jd.JobName, jd.URL)
	if jd.Status != prowapi.FailureState {
		log.Printf("%s: message did not signal a failure: %v\n", prefix, jd.Status)
		return false
	}
	// check type
	if jd.JobType != prowapi.PresubmitJob {
		log.Printf("%s: message did not originate from presubmit: %v\n", prefix, jd.JobType)
		return false
	}
	// check repo
	if len(jd.Refs) == 0 {
		log.Printf("%s: message does not contain any repository references\n", prefix)
		return false
	}
	repos, err := client.GetReportRepos(flakesRecorderJobName)
	if err != nil {
		log.Printf("%s: error getting reporter's repositories: %v\n", prefix, err)
		return false
	}
	expRepo := false
	for _, repo := range repos {
		if jd.Refs[0].Repo == repo {
			expRepo = true
			break
		}
	}
	if !expRepo {
		log.Printf("%s: message's repo is not being analyzed by flaky test reporter: '%v'\n", prefix, jd.Refs[0].Repo)
		return false
	}
	// make sure pull ID exists
	if len(jd.Refs[0].Pulls) == 0 {
		log.Printf("%s: message does not contain any pull requests\n", prefix)
		return false
	}
	return true
}

// getFailedTests gets all the tests that failed in the given job.
func (jd *JobData) getFailedTests() ([]string, error) {
	// use cache if it is populated
	if len(jd.failedTests) > 0 {
		return jd.failedTests, nil
	}
	job := prow.NewJob(jd.JobName, string(jd.JobType), jd.Refs[0].Repo, jd.Refs[0].Pulls[0].Number)
	// Check latest build instead of using jd.RunID, as there are times where
	// devs initiated retry manually before retryer gets to it, and in this case
	// scaning latest build can help retryer avoid initiating another retry
	// since latest build has no test failure yet
	buildID, err := job.GetLatestBuildNumber()
	if err != nil {
		return nil, err
	}
	build := job.NewBuild(buildID)
	results, err := GetCombinedResultsForBuild(build)
	if err != nil {
		return nil, err
	}
	var tests []string
	for _, suites := range results {
		for _, suite := range suites.Suites {
			for _, test := range suite.TestCases {
				if test.GetTestStatus() == junit.Failed {
					tests = append(tests, fmt.Sprintf("%s.%s", suite.Name, test.Name))
				}
			}
		}
	}
	jd.failedTests = tests
	return tests, nil
}

// TODO: This function is a direct copy-paste of the function in
// tools/flaky-test-reporter/result.go. Refactor it out into a shared library.

// GetCombinedResultsForBuild gets all junit results from a build,
// and converts each one into a junit TestSuites struct
func GetCombinedResultsForBuild(build *prow.Build) ([]*junit.TestSuites, error) {
	var allSuites []*junit.TestSuites
	for _, artifact := range build.GetArtifacts() {
		_, fileName := filepath.Split(artifact)
		if !strings.HasPrefix(fileName, "junit_") || !strings.HasSuffix(fileName, ".xml") {
			continue
		}
		relPath, _ := filepath.Rel(build.StoragePath, artifact)
		contents, err := build.ReadFile(relPath)
		if err != nil {
			return nil, err
		}
		if suites, err := junit.UnMarshal(contents); err != nil {
			return nil, err
		} else {
			allSuites = append(allSuites, suites)
		}
	}
	return allSuites, nil
}

// getFlakyTests gets the current flaky tests from the repo JobData originated from
func (jd *JobData) getFlakyTests() ([]string, error) {
	return client.GetFlakyTests(flakesRecorderJobName, jd.Refs[0].Repo)
}

// compareTests compares lists of failed and flaky tests, and returns any outlying failed
// tests, i.e. tests that failed that are NOT flaky.
func getNonFlakyTests(failedTests, flakyTests []string) []string {
	flakyMap := map[string]bool{}
	for _, flaky := range flakyTests {
		flakyMap[flaky] = true
	}
	var notFlaky []string
	for _, failed := range failedTests {
		if _, ok := flakyMap[failed]; !ok {
			notFlaky = append(notFlaky, failed)
		}
	}
	return notFlaky
}
