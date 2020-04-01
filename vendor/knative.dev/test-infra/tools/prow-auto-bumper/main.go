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

// prow-auto-bumper finds stable Prow components version used by k8s,
// and creates PRs updating them in knative/test-infra

package main

import (
	"flag"
	"log"

	"knative.dev/pkg/test/ghutil"

	"knative.dev/test-infra/pkg/git"
)

func main() {
	githubAccount := flag.String("github-account", "", "Token file for Github authentication")
	gitUserID := flag.String("git-userid", "", "The github ID of user for hosting fork, i.e. Github ID of bot")
	gitUserName := flag.String("git-username", "", "The username to use on the git commit. Requires --git-email")
	gitEmail := flag.String("git-email", "", "The email to use on the git commit. Requires --git-username")
	dryrun := flag.Bool("dry-run", false, "dry run switch")
	flag.Parse()

	if dryrun != nil && *dryrun {
		log.Println("Running in [dry run mode]")
	}

	gc, err := ghutil.NewGithubClient(*githubAccount)
	if err != nil {
		log.Fatalf("cannot authenticate to github: %v", err)
	}

	srcGI := git.Info{
		Org:    srcOrg,
		Repo:   srcRepo,
		Head:   srcPRHead,
		Base:   srcPRBase,
		UserID: srcPRUserID,
	}

	targetGI := git.Info{
		Org:      org,
		Repo:     repo,
		Head:     PRHead,
		Base:     PRBase,
		UserID:   *gitUserID,
		UserName: *gitUserName,
		Email:    *gitEmail,
	}

	gcw := &GHClientWrapper{gc}
	bestVersion, err := retryGetBestVersion(gcw, srcGI)
	if err != nil {
		log.Fatalf("cannot get best version from %s/%s: '%v'", srcGI.Org, srcGI.Repo, err)
	}
	log.Printf("Found version to update. Old Version: '%s', New Version: '%s'",
		bestVersion.dominantVersions.oldVersion, bestVersion.dominantVersions.newVersion)

	msgs, err := bestVersion.updateAllFiles(fileFilters, imageRegexp, *dryrun)
	if err != nil {
		log.Fatalf("failed updating files: '%v'", err)
	}

	if err = createOrUpdatePR(gcw, bestVersion, targetGI, msgs, *dryrun); err != nil {
		log.Fatalf("failed creating pullrequest: '%v'", err)
	}
}
