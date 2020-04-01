/*
Copyright 2020 The Knative Authors

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
	"flag"
	"log"
	"strings"

	"github.com/google/go-github/github"
	"knative.dev/pkg/test/ghutil"
	"knative.dev/pkg/test/prow"

	"knative.dev/test-infra/tools/prow-config-updater/config"
)

func main() {
	githubTokenPath := flag.String("github-token", "",
		"Github token file path for authenticating with Github")
	githubBotName := flag.String("github-bot-name", "knative-prow-robot",
		"Github bot name that is used in creating auto-merge PRs")
	flag.Parse()

	ec, err := prow.GetEnvConfig()
	if err != nil {
		log.Fatalf("Error getting environment variables for Prow: %v", err)
	}

	// We only check for presubmit jobs.
	if ec.JobType == prow.PresubmitJob {
		var err error
		gc, err := ghutil.NewGithubClient(*githubTokenPath)
		if err != nil {
			log.Printf("Error creating client with token %q: %v", *githubTokenPath, err)
			log.Printf("Proceeding with unauthenticated client")
			gc = &ghutil.GithubClient{Client: github.NewClient(nil)}
		}

		pn := ec.PullNumber
		org := ec.RepoOwner
		repo := ec.RepoName
		pr, err := gc.GetPullRequest(org, repo, int(pn))
		if err != nil {
			log.Fatalf("Cannot find the pull request %d: %v", int(pn), err)
		}

		// If the PR is created by the bot, skip the check.
		if *pr.User.Login == *githubBotName {
			return
		}

		files, err := gc.ListFiles(org, repo, int(pn))
		if err != nil {
			log.Fatalf("Cannot find files changed in this PR: %v", err)
		}

		// Collect all files that are not allowed to change directly by users.
		fns := make([]string, len(files))
		for i, f := range files {
			fns[i] = f.GetFilename()
		}
		bannedFiles := config.CollectRelevantFiles(fns, config.ProdProwKeyConfigPaths)

		// If any of the production Prow key config files are changed, report the error.
		if len(bannedFiles) != 0 {
			log.Fatalf(
				"Directly changing the production Prow cluster config and templates is not allowed, please revert:\n%s",
				strings.Join(bannedFiles, "\n"))
		}
	}
}
