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
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"k8s.io/apimachinery/pkg/util/wait"
	"knative.dev/pkg/test/cmd"
	"knative.dev/pkg/test/ghutil"
	"knative.dev/pkg/test/helpers"

	"knative.dev/test-infra/pkg/git"
	"knative.dev/test-infra/tools/prow-config-updater/config"
)

// GitHubMainHandler is used for the performing main operations on GitHub.
type GitHubMainHandler struct {
	client *ghutil.GithubClient
	info   git.Info
	dryrun bool
}

// Get the latest pull request created in this repository.
// This function is based on the assumption that all PRs are merged with "squash" and no force push is allowed,
// if the repository is not configured in this way, it will not work.
// TODO(chizhg): get rid of this hack once Prow supports setting PR number as an env var for postsubmit jobs.
func (gc *GitHubMainHandler) getLatestPullRequest() (*github.PullRequest, error) {
	// Use git command to get the latest commit ID.
	ci, err := cmd.RunCommand("git rev-parse HEAD")
	if err != nil {
		return nil, fmt.Errorf("error getting the last commit ID: %v", err)
	}
	// As we always use squash in merging PRs, we can get the pull request with the commit ID.
	pr, err := gc.client.GetPullRequestByCommitID(config.OrgName, config.RepoName, strings.TrimSpace(ci))
	if err != nil {
		return nil, fmt.Errorf("error getting the PR with commit ID %q: %v", strings.TrimSpace(ci), err)
	}
	return pr, nil
}

// Get the list of changed file names with the given PR number.
func (gc *GitHubMainHandler) getChangedFiles(pn int) ([]string, error) {
	fs, err := gc.client.ListFiles(config.OrgName, config.RepoName, pn)
	if err != nil {
		return nil, fmt.Errorf("error listing the changed files for PR %q: %v", pn, err)
	}

	fns := make([]string, len(fs))
	for i := range fns {
		fns[i] = *fs[i].Filename
	}

	return fns, nil
}

// Use the pull bot (https://github.com/wei/pull) to create a PR in the fork repository.
func (gc *GitHubMainHandler) createForkPullRequest(forkOrgName string) (*github.PullRequest, error) {
	// The endpoint to manually trigger pull bot to create the pull request in the fork.
	pullTriggerEndpoint := fmt.Sprintf(config.PullEndpointTemplate, forkOrgName, config.RepoName)
	resp, err := http.Get(pullTriggerEndpoint)
	if err != nil {
		return nil, fmt.Errorf("error creating the pull request in the fork repository: %v", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading the response for creating the pull request in the fork repository: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "Success" {
		return nil, fmt.Errorf("error creating the pull request in the fork repository: "+
			"status code is %d, body is %s", resp.StatusCode, body)
	}
	return gc.findForkPullRequest(forkOrgName)
}

// Find the pull request created by the pull bot (should be exactly one pull request, otherwise there must be an error)
func (gc *GitHubMainHandler) findForkPullRequest(forkOrgName string) (*github.PullRequest, error) {
	prs, _, err := gc.client.Client.PullRequests.List(
		context.Background(), forkOrgName, config.RepoName,
		&github.PullRequestListOptions{
			State: "open",
			Head:  "knative:master",
		})
	orgRepoName := fmt.Sprintf("%s/%s", forkOrgName, config.RepoName)
	if err != nil {
		return nil, fmt.Errorf("error listing pull request in repository %s: %v", orgRepoName, err)
	}
	var forkpr *github.PullRequest
	for _, pr := range prs {
		if *pr.User.Login == config.PullBotName {
			forkpr = pr
		}
	}
	if forkpr == nil {
		return nil, fmt.Errorf("expected one pull request in repository %s but found %d", orgRepoName, len(prs))
	}
	return forkpr, nil
}

// Wait for the fork pull request to be merged by polling its state.
func (gc *GitHubMainHandler) waitForForkPullRequestMerged(forkOrgName string, pn int) error {
	interval := 10 * time.Second
	timeout := 20 * time.Minute
	return helpers.Run(
		fmt.Sprintf("Wait until the fork pull request '%d' to merged", pn),
		func() error {
			return wait.PollImmediate(interval, timeout, func() (bool, error) {
				pr, err := gc.client.GetPullRequest(forkOrgName, config.RepoName, pn)
				if err != nil {
					return false, err
				}
				if pr.Merged == nil || !*pr.Merged {
					return false, nil
				}
				return true, nil
			})
		},
		gc.dryrun,
	)
}

// Create a pull request that can be automatically merged without manual approval.
func (gc *GitHubMainHandler) createAutoMergePullRequest(title, body string) (*github.PullRequest, error) {
	gi := gc.info
	var pr *github.PullRequest
	var err error
	err = helpers.Run(
		fmt.Sprintf("Committing the local changes and creating an auto-merge pull request %q", title),
		func() error {
			if err = git.MakeCommit(gi, title, gc.dryrun); err != nil {
				return fmt.Errorf("error creating a new Git commit: %v", err)
			}
			pr, err = gc.client.CreatePullRequest(gi.Org, gi.Repo, gi.GetHeadRef(), gi.Base, title, body)
			if err != nil {
				return fmt.Errorf("error creating the auto-merge pull request: %v", err)
			}
			if err = gc.client.AddLabelsToIssue(gi.Org, gi.Repo, *pr.Number, []string{config.AutoMergeLabel}); err != nil {
				return fmt.Errorf("error adding label on the auto-merge pull request: %v", err)
			}
			return nil
		},
		gc.dryrun,
	)
	return pr, err
}
