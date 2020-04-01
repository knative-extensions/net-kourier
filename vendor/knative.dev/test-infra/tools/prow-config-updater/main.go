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

	"knative.dev/pkg/test/ghutil"

	"knative.dev/test-infra/pkg/git"
	"knative.dev/test-infra/tools/prow-config-updater/config"
)

func main() {
	githubTokenFile := flag.String("github-token-file", "",
		"Token file for Github authentication, used for most of important interactions with Github")
	githubUserID := flag.String("github-userid", "",
		"The github ID of user for hosting fork, i.e. Github ID of bot")
	gitUserName := flag.String("git-username", "",
		"The username to use on the git commit. Requires --git-email")
	gitEmail := flag.String("git-email", "",
		"The email to use on the git commit. Requires --git-username")
	commentGithubTokenFile := flag.String("comment-github-token-file", "",
		"Token file for Github authentication, used for adding comments on Github pull requests")
	dryrun := flag.Bool("dry-run", false, "dry run switch")
	flag.Parse()

	mgc, err := ghutil.NewGithubClient(*githubTokenFile)
	if err != nil {
		log.Fatalf("Failed creating main github client: %v", err)
	}
	cgc, err := ghutil.NewGithubClient(*commentGithubTokenFile)
	if err != nil {
		log.Fatalf("Failed creating commenter github client: %v", err)
	}

	cli := &Client{
		githubmainhandler: &GitHubMainHandler{client: mgc, dryrun: *dryrun, info: git.Info{
			Org:      config.OrgName,
			Repo:     config.RepoName,
			Head:     config.PRHead,
			Base:     config.PRBase,
			UserID:   *githubUserID,
			UserName: *gitUserName,
			Email:    *gitEmail,
		}},
		githubcommenter: &GitHubCommenter{client: cgc, dryrun: *dryrun},
		// The forkOrgName is the same as the Git user ID we use in this tool.
		forkOrgName: *githubUserID,
		dryrun:      *dryrun,
	}
	if err := cli.initialize(); err != nil {
		log.Fatalf("Failed intializing the client: %v", err)
	}

	if err := cli.runProwConfigUpdate(); err != nil {
		log.Fatalf("Failed updating Prow configs: %v", err)
	}
}
