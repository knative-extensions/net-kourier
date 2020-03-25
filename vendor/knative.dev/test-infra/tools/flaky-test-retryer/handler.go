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

// handler.go contains most of the main logic for the flaky-test-retryer. Listen for
// incoming Pubsub messages, verify that the message we received is one we want to
// process, compare flaky and failed tests, and trigger retests if necessary.

package main

import (
	"context"
	"fmt"
	"log"

	"knative.dev/pkg/test/ghutil"

	"knative.dev/test-infra/tools/monitoring/subscriber"
	// TODO: remove this import once "k8s.io/test-infra" import problems are fixed
	// https://github.com/test-infra/test-infra/issues/912
	"knative.dev/test-infra/tools/monitoring/prowapi"
)

const pubsubTopic = "flaky-test-retryer"

// HandlerClient wraps the other clients we need when processing failed jobs.
type HandlerClient struct {
	context.Context
	pubsub *subscriber.Client
	github *GithubClient
}

// NewHandlerClient gives us a handler where we can listen for Pubsub messages and
// post comments on GitHub.
func NewHandlerClient(serviceAccount, githubAccount string, dryrun bool) (*HandlerClient, error) {
	ctx := context.Background()
	if err := InitLogParser(serviceAccount); err != nil {
		log.Fatalf("Failed authenticating GCS: '%v'", err)
	}
	githubClient, err := NewGithubClient(githubAccount, dryrun)
	if err != nil {
		return nil, fmt.Errorf("Github client: %v", err)
	}
	pubsubClient, err := subscriber.NewSubscriberClient(pubsubTopic)
	if err != nil {
		return nil, fmt.Errorf("Pubsub client: %v", err)
	}
	return &HandlerClient{
		ctx,
		pubsubClient,
		githubClient,
	}, nil
}

// Listen scans for incoming Pubsub messages, spawning a new goroutine for each
// one that fits our criteria.
func (hc *HandlerClient) Listen() {
	log.Printf("Listening for failed jobs...\n")
	for {
		log.Println("Starting ReceiveMessageAckAll")
		hc.pubsub.ReceiveMessageAckAll(context.Background(), func(msg *prowapi.ReportMessage) {
			log.Printf("Message received for %q", msg.URL)
			data := &JobData{msg, nil, nil}
			if data.IsSupported() {
				go hc.HandleJob(data)
			}
		})
		log.Println("Done with previous ReceiveMessageAckAll call")
	}
}

// HandleJob gets the job's failed tests and the current flaky tests,
// compares them, and triggers a retest if all the failed tests are flaky.
func (hc *HandlerClient) HandleJob(jd *JobData) {
	logWithPrefix(jd, "fit all criteria - Starting analysis\n")

	pull, err := hc.github.GetPullRequest(jd.Refs[0].Org, jd.Refs[0].Repo, jd.Refs[0].Pulls[0].Number)
	if err != nil {
		logWithPrefix(jd, "could not get Pull Request: %v", err)
		return
	}

	if *pull.State != string(ghutil.PullRequestOpenState) {
		logWithPrefix(jd, "Pull Request is not open: %q", *pull.State)
		return
	}

	failedTests, err := jd.getFailedTests()
	if err != nil {
		logWithPrefix(jd, "could not get failed tests: %v", err)
		return
	}
	if len(failedTests) == 0 {
		logWithPrefix(jd, "no failed tests, skipping\n")
		return
	}
	logWithPrefix(jd, "got %d failed tests", len(failedTests))

	flakyTests, err := jd.getFlakyTests()
	if err != nil {
		logWithPrefix(jd, "could not get flaky tests: %v", err)
		return
	}
	logWithPrefix(jd, "got %d flaky tests from today's report\n", len(flakyTests))

	outliers := getNonFlakyTests(failedTests, flakyTests)
	if err := hc.github.PostComment(jd, outliers); err != nil {
		logWithPrefix(jd, "Could not post comment: %v", err)
	}
}

// logWithPrefix wraps a call to log.Printf, prefixing the arguments with details
// about the job passed in.
func logWithPrefix(jd *JobData, format string, a ...interface{}) {
	input := append([]interface{}{jd.Refs[0].Repo, jd.Refs[0].Pulls[0].Number, jd.JobName, jd.RunID}, a...)
	log.Printf("%s/pull/%d: %s/%s: "+format, input...)
}
