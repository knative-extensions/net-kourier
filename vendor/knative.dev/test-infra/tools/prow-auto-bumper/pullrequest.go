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

// pullrequest.go creates git commits and Pull Requests

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/test/ghutil"
	"knative.dev/pkg/test/helpers"

	"knative.dev/test-infra/shared/git"
)

func generatePRBody(extraMsgs []string) string {
	var body string
	if len(extraMsgs) > 0 {
		body += "Info:\n"
		for _, msg := range sets.NewString(extraMsgs...).List() {
			body += fmt.Sprintf("%s\n", msg)
		}
		body += "\n"
	}

	body += "\nPlease check [Prow release notes]" +
		"(https://github.com/kubernetes/test-infra/blob/master/prow/ANNOUNCEMENTS.md) " +
		"to make sure there are no breaking changes.\n"

	oncaller, err := getOncaller()
	var assignment string
	if err == nil {
		if oncaller != "" {
			assignment = fmt.Sprintf("/assign @%s\n/cc @%s\n", oncaller, oncaller)
		} else {
			assignment = "Nobody is currently oncall."
		}
	} else {
		assignment = fmt.Sprintf("An error occurred while finding an assignee: `%v`.", err)
	}

	return body + assignment
}

func getOncaller() (string, error) {
	req, err := http.Get(oncallAddress)
	if err != nil {
		return "", err
	}
	defer req.Body.Close()
	if req.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error %d (%q) fetching current oncaller", req.StatusCode, req.Status)
	}
	oncall := struct {
		Oncall struct {
			ToolsInfra string `json:"tools-infra"`
		} `json:"Oncall"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&oncall); err != nil {
		return "", err
	}
	return oncall.Oncall.ToolsInfra, nil
}

// Get existing open PR not merged yet
func getExistingPR(gcw *GHClientWrapper, gi git.Info, matchTitle string) (*github.PullRequest, error) {
	var res *github.PullRequest
	PRs, err := gcw.ListPullRequests(gi.Org, gi.Repo, gi.GetHeadRef(), gi.Base)
	if err == nil {
		for _, PR := range PRs {
			if string(ghutil.PullRequestOpenState) == *PR.State && strings.Contains(*PR.Title, matchTitle) {
				res = PR
				break
			}
		}
	}
	return res, err
}

func createOrUpdatePR(gcw *GHClientWrapper, pv *PRVersions, gi git.Info, extraMsgs []string, dryrun bool) error {
	vs := pv.getDominantVersions()
	commitMsg := fmt.Sprintf("Update prow from %s to %s, and other images as necessary.", vs.oldVersion, vs.newVersion)
	matchTitle := "Update prow to"
	title := fmt.Sprintf("%s %s", matchTitle, vs.newVersion)
	body := generatePRBody(extraMsgs)
	if err := git.MakeCommit(gi, commitMsg, dryrun); err != nil {
		return fmt.Errorf("failed git commit: '%v'", err)
	}
	existPR, err := getExistingPR(gcw, gi, matchTitle)
	if err != nil {
		return fmt.Errorf("failed querying existing pullrequests: '%v'", err)
	}
	if existPR != nil {
		log.Printf("Found open PR '%d'", *existPR.Number)
		return helpers.Run(
			fmt.Sprintf("Updating PR '%d', title: '%s', body: '%s'", *existPR.Number, title, body),
			func() error {
				if _, err := gcw.EditPullRequest(gi.Org, gi.Repo, *existPR.Number, title, body); err != nil {
					return fmt.Errorf("failed updating pullrequest: '%v'", err)
				}
				return nil
			},
			dryrun,
		)
	}
	return helpers.Run(
		fmt.Sprintf("Creating PR, title: '%s', body: '%s'", title, body),
		func() error {
			if _, err := gcw.CreatePullRequest(gi.Org, gi.Repo, gi.GetHeadRef(), gi.Base, title, body); err != nil {
				return fmt.Errorf("failed creating pullrequest: '%v'", err)
			}
			return nil
		},
		dryrun,
	)
}
