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
	"strings"
	"testing"

	"github.com/google/go-github/github"
	"knative.dev/pkg/test/ghutil/fakeghutil"
	"knative.dev/test-infra/tools/flaky-test-reporter/config"
)

var testStatsMapForTest = map[string]TestStat{
	"passed": {
		TestName: "a",
		Passed:   []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		Failed:   []int{},
		Skipped:  []int{},
	},
	"flaky": {
		TestName: "a",
		Passed:   []int{1, 2, 3, 4, 5, 6, 7, 8, 9},
		Failed:   []int{0},
		Skipped:  []int{},
	},
	"failed": {
		TestName: "a",
		Passed:   []int{},
		Failed:   []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		Skipped:  []int{},
	},
	"notenoughdata": {
		TestName: "a",
		Passed:   []int{0, 1, 2, 3, 4, 5, 6},
		Failed:   []int{},
		Skipped:  []int{7, 8, 9},
	},
}

var (
	fakeOrg    = "fakeorg"
	fakeRepo   = "fakerepo"
	fakeUserID = int64(99)
	fakeUser   = &github.User{
		ID: &fakeUserID,
	}
	dryrun = false
)

func getFakeGithubIssueHandler() *GithubIssueHandler {
	fg := fakeghutil.NewFakeGithubClient()
	fg.Repos = []string{fakeRepo}
	fg.User = fakeUser
	return &GithubIssueHandler{
		user:   fakeUser,
		client: fg,
	}
}

func createNewIssue(fgih *GithubIssueHandler, title, body, testStat string) (*github.Issue, *github.IssueComment) {
	issue, _ := fgih.client.CreateIssue(fakeOrg, fakeRepo, title, body)
	commentBody := fmt.Sprintf("Latest result for this test: %s", testStat)
	comment, _ := fgih.client.CreateComment(fakeOrg, fakeRepo, *issue.Number, commentBody)
	return issue, comment
}

func createRepoData(passed, flaky, failed, notenoughdata int, issueRepo string, startTime int64) RepoData {
	cfg := config.JobConfig{
		Repo:      fakeRepo,
		IssueRepo: issueRepo,
	}
	tss := map[string]*TestStat{}
	for status, count := range map[string]int{
		"passed":        passed,
		"flaky":         flaky,
		"failed":        failed,
		"notenoughdata": notenoughdata,
	} {
		for i := 0; i < count; i++ {
			ts := testStatsMapForTest[status]
			tss[fmt.Sprintf("test%s_%d", status, i)] = &ts
		}
	}
	return RepoData{
		Config:             cfg,
		TestStats:          tss,
		LastBuildStartTime: &startTime,
	}
}

func TestCreateIssue(t *testing.T) {
	datas := []struct {
		passed, flaky, failed, notenoughdata int
		issueRepo                            string
		wantIssues                           int
	}{
		{197, 6, 0, 0, fakeRepo, 1}, // flaky rate > 1% and > 5 flaky tests, create only 1 issue
		{197, 2, 0, 0, fakeRepo, 2}, // flaky rate > 1% and < 5 flaky tests, create issue for each
		{200, 2, 0, 0, fakeRepo, 2}, // flaky rate < 1%, create issue for each
		{197, 2, 0, 0, "", 0},       // flaky rate > 1%, flag is set to not create issue
		{200, 2, 0, 0, "", 0},       // flaky rate < 1%, flag is set to not create issue

	}

	for _, d := range datas {
		fgih := getFakeGithubIssueHandler()
		repoData := createRepoData(d.passed, d.flaky, d.failed, d.notenoughdata, d.issueRepo, int64(0))
		fgih.processGithubIssuesForRepo(repoData, make(map[string][]flakyIssue), dryrun)
		issues, _ := fgih.client.ListIssuesByRepo(fakeOrg, fakeRepo, []string{})
		if len(issues) != d.wantIssues {
			t.Fatalf("2%% tests failed, got %d issues, want %d issue", len(issues), d.wantIssues)
		}
	}
}

func TestExistingIssue(t *testing.T) {
	fgih := getFakeGithubIssueHandler()
	repoData := createRepoData(200, 2, 0, 0, fakeRepo, int64(0))
	flakyIssuesMap, _ := fgih.getFlakyIssues()
	fgih.processGithubIssuesForRepo(repoData, flakyIssuesMap, dryrun)
	existIssues, _ := fgih.client.ListIssuesByRepo(fakeOrg, fakeRepo, []string{})
	flakyIssuesMap, _ = fgih.getFlakyIssues()

	*repoData.LastBuildStartTime++
	fgih.processGithubIssuesForRepo(repoData, flakyIssuesMap, dryrun)
	issues, _ := fgih.client.ListIssuesByRepo(fakeOrg, fakeRepo, []string{})
	if len(existIssues) != len(issues) {
		t.Fatalf("issues already exists, got %d new issues, want 0 new issues", len(issues)-len(existIssues))
	}
}

func TestUpdateIssue(t *testing.T) {
	dataForTest := []struct {
		issueState     string
		ts             TestStat
		passedLastTime bool
		appendComment  bool
		wantStatus     string
		wantErr        error
	}{
		{"open", testStatsMapForTest["flaky"], false, true, "open", nil},
		{"open", testStatsMapForTest["flaky"], true, true, "open", nil},
		{"open", testStatsMapForTest["passed"], false, true, "open", nil},
		{"open", testStatsMapForTest["passed"], true, true, "closed", nil},
		{"open", testStatsMapForTest["failed"], false, true, "open", nil},
		{"open", testStatsMapForTest["failed"], true, true, "open", nil},
		{"open", testStatsMapForTest["notenoughdata"], false, true, "open", nil},
		{"open", testStatsMapForTest["notenoughdata"], true, true, "open", nil},
		{"closed", testStatsMapForTest["flaky"], false, true, "open", nil},
		{"closed", testStatsMapForTest["flaky"], true, true, "open", nil},
		{"closed", testStatsMapForTest["passed"], false, true, "closed", nil},
		{"closed", testStatsMapForTest["passed"], true, true, "closed", nil},
		{"closed", testStatsMapForTest["failed"], false, true, "closed", nil},
		{"closed", testStatsMapForTest["failed"], true, true, "closed", nil},
		{"closed", testStatsMapForTest["notenoughdata"], false, true, "closed", nil},
		{"closed", testStatsMapForTest["notenoughdata"], true, true, "closed", nil},
	}

	title := "fake title"
	body := "fake body"

	for _, data := range dataForTest {
		fgih := getFakeGithubIssueHandler()
		var issue *github.Issue
		var comment *github.IssueComment
		if data.passedLastTime {
			issue, comment = createNewIssue(fgih, title, body, "Passed")
		} else {
			issue, comment = createNewIssue(fgih, title, body, "Flaky")
		}
		commentBody := comment.GetBody()

		fi := flakyIssue{
			issue:   issue,
			comment: comment,
		}

		gotErr := fgih.updateIssue(fi, "new", &data.ts, dryrun)
		if data.wantErr == nil {
			if gotErr != nil {
				t.Fatalf("update %v, got err: '%v', want err: '%v'", data, gotErr, data.wantErr)
			}
		} else {
			if !strings.HasPrefix(gotErr.Error(), data.wantErr.Error()) {
				t.Fatalf("update %v, got err start with: '%s', want err: '%s'", data, gotErr.Error(), data.wantErr.Error())
			}
		}

		gotComment, _ := fgih.client.GetComment(fakeOrg, fakeRepo, *comment.ID)
		if data.appendComment && gotComment.GetBody() == commentBody {
			t.Fatalf("update comment %v, got: '%s', want: 'new' on top of existing comment", data, gotComment.GetBody())
		}
		if !data.appendComment && gotComment.GetBody() != commentBody {
			t.Fatalf("update comment %v, got: '%s', want: '%s'", data, gotComment.GetBody(), commentBody)
		}
	}
}
