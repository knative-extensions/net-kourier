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

// flaky-test-retryer detects failed integration jobs on new pull requests,
// determines if they failed due to flaky tests, posts comments describing the
// issue, and retries them until they succeed.

package main

import (
	"flag"
	"log"
	"os"
)

const (
	flakesRecorderJobName = "ci-knative-flakes-resultsrecorder"
)

type EnvFlags struct {
	ServiceAccount string // GCP service account file path
	GithubAccount  string // github account file path
	Dryrun         bool   // dry run toggle
}

func initFlags() *EnvFlags {
	var f EnvFlags
	defaultServiceAccount := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	flag.StringVar(&f.ServiceAccount, "service-account", defaultServiceAccount, "JSON key file for GCS service account")
	flag.StringVar(&f.GithubAccount, "github-account", "", "Token file for Github authentication")
	flag.BoolVar(&f.Dryrun, "dry-run", false, "dry run switch")
	flag.Parse()
	return &f
}

func main() {
	flags := initFlags()

	handler, err := NewHandlerClient(flags.ServiceAccount, flags.GithubAccount, flags.Dryrun)
	if err != nil {
		log.Fatalf("Coud not create handler: '%v'", err)
	}

	if flags.Dryrun {
		log.Println("running in [dry run] mode")
	}

	handler.Listen()
}
