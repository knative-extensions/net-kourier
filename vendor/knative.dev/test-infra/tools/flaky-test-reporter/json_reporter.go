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

package main

import (
	"fmt"
	"log"
	"sync"

	"knative.dev/pkg/test/helpers"
	"knative.dev/test-infra/tools/flaky-test-reporter/jsonreport"
)

// when reporting on all flaky tests in a repo, we want to eliminate the "job" layer, compressing all flaky
// tests in that repo into a single list. There can be duplicate tests across jobs, though, so we store tests
// in a nested map first to eliminate those duplicates.
func getFlakyTestSet(repoDataAll []RepoData) map[string]map[string]bool {
	// this map represents "repo: test: exists"
	flakyTestSet := map[string]map[string]bool{}
	for _, rd := range repoDataAll {
		if flakyTestSet[rd.Config.Repo] == nil {
			flakyTestSet[rd.Config.Repo] = map[string]bool{}
		}
		for _, test := range getFlakyTests(rd) {
			flakyTestSet[rd.Config.Repo][test] = true
		}
	}
	return flakyTestSet
}

func writeFlakyTestsToJSON(repoDataAll []RepoData, dryrun bool) error {
	client := &jsonreport.JSONClient{}
	var allErrs []error
	flakyTestSets := getFlakyTestSet(repoDataAll)
	ch := make(chan bool, len(flakyTestSets))
	wg := sync.WaitGroup{}
	for repo := range flakyTestSets {
		wg.Add(1)
		go func(wg *sync.WaitGroup, repo string) {
			testSet := flakyTestSets[repo]
			var testList []string
			for test := range testSet {
				testList = append(testList, test)
			}
			if err := helpers.Run(
				fmt.Sprintf("writing JSON report for repo '%s'", repo),
				func() error {
					_, err := client.CreateReport(repo, testList, true)
					return err
				},
				dryrun); err != nil {
				allErrs = append(allErrs, err)
				log.Printf("failed writing JSON report for repo '%s': '%v'", repo, err)
			}
			if dryrun {
				log.Printf("[dry run] JSON report not written to bucket\n")
			}
			ch <- true
			wg.Done()
		}(&wg, repo)
	}
	wg.Wait()
	close(ch)
	return helpers.CombineErrors(allErrs)
}
