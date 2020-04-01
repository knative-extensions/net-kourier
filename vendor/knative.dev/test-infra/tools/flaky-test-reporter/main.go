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

// flaky-test-reporter collects test results from continuous flows,
// identifies flaky tests, tracking flaky tests related github issues,
// and sends slack notifications.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"knative.dev/pkg/test/helpers"
	"knative.dev/pkg/test/slackutil"
	"knative.dev/test-infra/pkg/prow"
	"knative.dev/test-infra/tools/flaky-test-reporter/config"
)

var (
	// Builds to be analyzed, this is determined by flag
	buildsCount int
	// Minimal number of results to be counted as valid results for each
	// testcase, this is derived from buildsCount and requiredRatio
	requiredCount float32
)

func main() {
	serviceAccount := flag.String("service-account", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), "JSON key file for GCS service account")
	githubAccount := flag.String("github-account", "", "Token file for Github authentication")
	slackAccount := flag.String("slack-account", "", "slack secret file for authenticating with Slack")
	buildsCountOverride := flag.Int("build-count", 10, "count of builds to scan")
	skipReport := flag.Bool("skip-report", false, "skip Github and Slack report")
	dryrun := flag.Bool("dry-run", false, "dry run switch")
	flag.Parse()

	buildsCount = *buildsCountOverride
	requiredCount = requiredRatio * float32(buildsCount)

	if *dryrun {
		log.Printf("running in [dry run mode]")
	}

	if err := prow.Initialize(*serviceAccount); err != nil { // Explicit authenticate with gcs Client
		log.Fatalf("Failed authenticating GCS: '%v'", err)
	}

	var repoDataAll []RepoData
	// Clean up local artifacts directory, this will be used later for artifacts uploads
	err := os.RemoveAll(prow.GetLocalArtifactsDir()) // this function returns nil if path not found
	if err != nil {
		log.Fatalf("Failed removing local artifacts directory: %v", err)
	}
	var jobErrs []error
	for _, jc := range config.JobConfigs {
		log.Printf("collecting results for job '%s' in repo '%s'\n", jc.Name, jc.Repo)
		rd, err := collectTestResultsForRepo(jc)
		if err != nil {
			err = fmt.Errorf("WARNING: error collecting results for job '%s' in repo '%s': %v", jc.Name, jc.Repo, err)
			log.Printf("%v", err)
			jobErrs = append(jobErrs, err)
			continue
		}
		if rd.LastBuildStartTime == nil {
			log.Printf("WARNING: no build found, skipping '%s' in repo '%s'", jc.Name, jc.Repo)
			continue
		}
		if err = createArtifactForRepo(*rd); err != nil {
			log.Fatalf("Error creating artifacts for job '%s' in repo '%s': %v", jc.Name, jc.Repo, err)
		}
		repoDataAll = append(repoDataAll, *rd)
	}

	// Errors that could result in inaccuracy reporting would be treated with fast fail by processGithubIssues,
	// so any errors returned are github opeations error, which in most cases wouldn't happen, but in case it
	// happens, it should fail the job after Slack notification
	jobErr := helpers.CombineErrors(jobErrs)
	jsonErr := writeFlakyTestsToJSON(repoDataAll, *dryrun)

	var ghErr, slackErr error
	var flakyIssues map[string][]flakyIssue

	if *skipReport {
		log.Printf("--skip-report provided, skipping Github and Slack report")
	} else {
		flakyIssues, ghErr = githubOperations(*githubAccount, repoDataAll, *dryrun)
		slackErr = slackOperations(*slackAccount, repoDataAll, flakyIssues, *dryrun)
	}

	if jobErr != nil {
		log.Printf("Job step failures:\n%v", jobErr)
	}
	if slackErr != nil {
		log.Printf("Slack step failures:\n%v", slackErr)
	}
	if jsonErr != nil {
		log.Printf("JSON step failures:\n%v", jsonErr)
	}
	// Fail this job if there is any error
	if jobErr != nil || jsonErr != nil || ghErr != nil || slackErr != nil {
		os.Exit(1)
	}
}

func githubOperations(ghToken string, repoData []RepoData, dryrun bool) (map[string][]flakyIssue, error) {
	gih, err := Setup(ghToken)
	if err != nil {
		return nil, err
	}

	return gih.processGithubIssues(repoData, dryrun)
}

func isWeekend(t time.Time) bool {
	weekDay := t.Weekday()
	return weekDay == time.Saturday || weekDay == time.Sunday
}

func slackOperations(slackToken string, repoData []RepoData, flakyIssues map[string][]flakyIssue, dryrun bool) error {
	if isWeekend(time.Now()) {
		log.Print("Skip Slack notification on weekend")
		return nil
	}

	// Verify that there are issues to notify on.
	if len(flakyIssues) == 0 {
		return nil
	}

	client, err := slackutil.NewWriteClient(knativeBotName, slackToken)
	if err != nil && !dryrun { // Dryrun doesn't do any Slack operation
		return err
	}

	return sendSlackNotifications(repoData, client, flakyIssues, dryrun)
}

func jsonOperations(repoData []RepoData, dryrun bool) error {
	return writeFlakyTestsToJSON(repoData, dryrun)
}
