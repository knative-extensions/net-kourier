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

// result.go contains structs and functions for shared data

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"knative.dev/test-infra/shared/common"
	"knative.dev/test-infra/shared/junit"
	"knative.dev/test-infra/shared/prow"
	"knative.dev/test-infra/tools/flaky-test-reporter/config"
)

const (
	flakyStatus    = "Flaky"
	passedStatus   = "Passed"
	lackDataStatus = "NotEnoughData"
	failedStatus   = "Failed"
)

// RepoData struct contains all configurations and test results for a repo
type RepoData struct {
	Config             config.JobConfig
	TestStats          map[string]*TestStat // key is test full name
	BuildIDs           []int                // all build IDs scanned in this run
	LastBuildStartTime *int64               // timestamp, determines how fresh the data is
}

// TestStat represents test results of a single testcase across all builds,
// Passed, Skipped and Failed contains buildIDs with corresponding results
type TestStat struct {
	TestName string
	Passed   []int
	Skipped  []int
	Failed   []int
}

func (ts *TestStat) isFlaky() bool {
	// This is only responsible for creating and reopening issue,
	// can be aggressive even when there is not enough runs.
	// For example  if there are 10 runs, 1 failed, 1 passed, 8 skipped,
	// this should still be considered flaky
	return len(ts.Failed) > 0 && len(ts.Passed) != 0
}

func (ts *TestStat) isPassed() bool {
	// This is responsible for marking issue as fixed, needs to be
	// very strict in terms of runs, so enforcing hasEnoughRuns here
	return ts.hasEnoughRuns() && len(ts.Failed) == 0
}

func (ts *TestStat) hasEnoughRuns() bool {
	return float32(len(ts.Passed)+len(ts.Failed)) >= requiredCount
}

func (ts *TestStat) getTestStatus() string {
	switch {
	case ts.isFlaky():
		return flakyStatus
	case ts.isPassed():
		return passedStatus
	case !ts.hasEnoughRuns():
		return lackDataStatus
	default:
		return failedStatus
	}
}

func getFlakyTests(rd RepoData) []string {
	var flakyTests []string
	for testName, ts := range rd.TestStats {
		if ts.isFlaky() {
			flakyTests = append(flakyTests, testName)
		}
	}
	return flakyTests
}

func getFlakyRate(rd RepoData) float32 {
	totalCount := len(rd.TestStats)
	if 0 == totalCount {
		return 0.0
	}
	return float32(len(getFlakyTests(rd))) / float32(totalCount)
}

func flakyRateAboveThreshold(rd RepoData) bool {
	// if the percent determined by the test count threshold is higher than
	// the percent threshold, use that instead of the percent threshold
	totalCount := len(rd.TestStats)
	if totalCount == 0 {
		return true
	}
	threshold := float32(countThreshold) / float32(totalCount)
	if percentThreshold > threshold {
		threshold = percentThreshold
	}
	return getFlakyRate(rd) > threshold
}

// createArtifactForRepo marshals RepoData into json format and stores it in a json file,
// under local artifacts directory
func createArtifactForRepo(rd RepoData) error {
	artifactsDir := prow.GetLocalArtifactsDir()
	err := common.CreateDir(path.Join(artifactsDir, rd.Config.Repo))
	if err != nil {
		return err
	}
	outFilePath := path.Join(artifactsDir, rd.Config.Repo, rd.Config.Name+".json")
	contents, err := json.Marshal(rd)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outFilePath, contents, 0644)
}

// addSuiteToRepoData adds all testCase from suite into RepoData
func addSuiteToRepoData(suite *junit.TestSuite, buildID int, rd *RepoData) {
	if rd.TestStats == nil {
		rd.TestStats = make(map[string]*TestStat)
	}
	for _, testCase := range suite.TestCases {
		testFullName := fmt.Sprintf("%s.%s", suite.Name, testCase.Name)
		if _, ok := rd.TestStats[testFullName]; !ok {
			rd.TestStats[testFullName] = &TestStat{TestName: testFullName}
		}
		switch testCase.GetTestStatus() {
		case junit.Passed:
			rd.TestStats[testFullName].Passed = append(rd.TestStats[testFullName].Passed, buildID)
		case junit.Skipped:
			rd.TestStats[testFullName].Skipped = append(rd.TestStats[testFullName].Skipped, buildID)
		case junit.Failed:
			rd.TestStats[testFullName].Failed = append(rd.TestStats[testFullName].Failed, buildID)
		}
	}
}

// TODO: This function has been directly copy-pasted into tools/flaky-test-retryer/log_parser.go
// Refactor it out into a shared library.

// getCombinedResultsForBuild gets all junit results from a build,
// and converts each one into a junit TestSuites struct
func getCombinedResultsForBuild(build *prow.Build) ([]*junit.TestSuites, error) {
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

// collectTestResultsForRepo collects test results, build IDs from all builds,
// as well as LastBuildStartTime, and stores them in RepoData
func collectTestResultsForRepo(jc config.JobConfig) (*RepoData, error) {
	rd := &RepoData{Config: jc}
	job := prow.NewJob(jc.Name, jc.Type, jc.Repo, 0)
	if !job.PathExists() {
		return rd, fmt.Errorf("job path not exist '%s'", jc.Name)
	}
	builds := getLatestFinishedBuilds(job, buildsCount)

	log.Printf("latest builds: ")
	for i, build := range builds {
		log.Printf("\t%d", build.BuildID)
		rd.BuildIDs = append(rd.BuildIDs, build.BuildID)
		if 0 == i { // This is the latest build as builds are sorted by start time in descending order
			rd.LastBuildStartTime = build.StartTime
		}
		combinedResults, err := getCombinedResultsForBuild(&build)
		if err != nil {
			return nil, err
		}
		for _, suites := range combinedResults {
			for _, suite := range suites.Suites {
				addSuiteToRepoData(&suite, build.BuildID, rd)
			}
		}
	}
	return rd, nil
}

func (rd *RepoData) getResultSliceForTest(testName string) []junit.TestStatusEnum {
	res := make([]junit.TestStatusEnum, len(rd.BuildIDs), len(rd.BuildIDs))
	ts := rd.TestStats[testName]
	for i, buildID := range rd.BuildIDs {
		switch {
		case intSliceContains(ts.Failed, buildID):
			res[i] = junit.Failed
		case intSliceContains(ts.Passed, buildID):
			res[i] = junit.Passed
		default:
			res[i] = junit.Skipped
		}
	}
	return res
}

func intSliceContains(its []int, target int) bool {
	for _, it := range its {
		if it == target {
			return true
		}
	}
	return false
}

// getLatestFinishedBuilds is an inexpensive way of listing latest finished builds, in comparing to
// the GetLatestBuilds function from prow package, as it doesn't precompute start/finish time before sorting.
// This function takes the assumption that build IDs are always incremental integers, it would fail if it doesn't
func getLatestFinishedBuilds(job *prow.Job, count int) []prow.Build {
	var builds []prow.Build
	buildIDs := job.GetBuildIDs()
	sort.Sort(sort.Reverse(sort.IntSlice(buildIDs)))
	for _, buildID := range buildIDs {
		if len(builds) >= count {
			break
		}
		build := job.NewBuild(buildID)
		if build.FinishTime != nil {
			if build.StartTime == nil {
				log.Fatalf("Failed parsing start time for finished build '%s'", build.StoragePath)
			}
			builds = append(builds, *build)
		}
	}
	if !sort.SliceIsSorted(builds, func(i, j int) bool {
		return *builds[i].StartTime > *builds[j].StartTime
	}) {
		log.Fatalf("Error: found build with smaller buildID started later than one with larger buildID")
	}
	return builds
}
