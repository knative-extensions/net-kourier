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
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/google/go-github/github"

	"knative.dev/pkg/test/ghutil"
	"knative.dev/pkg/test/helpers"
	"knative.dev/test-infra/shared/junit"
)

const (
	// flakyLabel is the Github issue label used for querying all flaky issues auto-generated.
	flakyLabel          = "auto:flaky"
	testIdentifierToken = "DONT_MODIFY_TEST_IDENTIFIER"
	latestStatusToken   = "Latest result for this test: "
	beforeHistoryToken  = "<!------Latest History of Up To 10 runs------>"
	afterHistoryToken   = "<!------End of History------>"
	jobLogsURL          = "https://prow.knative.dev/view/gcs/knative-prow/logs/"
	daysConsiderOld     = 30 // arbitrary number of days for an issue to be considered old
	maxHistoryEntries   = 10 // max count of history runs to show in unicode graph

	passedUnicode  = "&#10004;" // Checkmark unicode
	failedUnicode  = "&#10006;" // Cross unicode
	skippedUnicode = "&#9723;"  // Open square unicode

	// collapseTemplate is for creating collapsible format in comment, so that only latest result is expanded.
	// Github markdown supports collapse, strings between <summary></summary> is the title,
	// contents between <p></p> is collapsible body, body is hidden unless the title is clicked.
	collapseTemplate = `
<details>
	<summary>%s</summary><p>

%s
</p></details>`  // The blank line is crucial for collapsible format

	// issueBodyTemplate is a template for issue body
	issueBodyTemplate = `
### Auto-generated issue tracking flakiness of test
* **Test name**: %s
* **Repository name**: %s

<!-------------End of issue body, Please don't edit below this line------------->
<!--%s-->`
)

var (
	// testIdentifierPattern is used for formatting test identifier,
	// expect an argument of test identifier
	testIdentifierPattern = fmt.Sprintf("[%[1]s]%%s[%[1]s]", testIdentifierToken)
	// regex matching pattern for capturing testname
	reTestIdentifierRegex = regexp.MustCompile(fmt.Sprintf(`\[%[1]s\](.*?)\[%[1]s\]`, testIdentifierToken))

	// historyPattern is for creating history section in commment,
	// expect an argument of history Unicode from previous comment
	historyPattern = fmt.Sprintf("\n%s%%s%s\n%s Passed\t%s Failed\t%s Skipped\n",
		beforeHistoryToken, afterHistoryToken, passedUnicode, failedUnicode, skippedUnicode)
	// reHistory is for identifying history from comment
	reHistory = fmt.Sprintf("(?s)%s(.*?)%s", beforeHistoryToken, afterHistoryToken)
	// regex for identifying each single record
	reSingleRecordRegex = regexp.MustCompile(`[0-9]{4}\-[0-9]{2}\-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2} [+\-][0-9]{4} [A-Z]{3}:.*`)

	// latestStatusPattern is for creating latest test result line in comment,
	// expect an argument of test status as defined in result.go
	latestStatusPattern = fmt.Sprintf(`%s%%s`, latestStatusToken)

	// regex for identifying latest test result from comment
	reLatestStatusRegex = regexp.MustCompile(fmt.Sprintf(`%s([a-zA-Z]*)`, latestStatusToken))

	// Precompute timeConsiderOld so that the same standard used everywhere
	timeConsiderOld = time.Now().AddDate(0, 0, -daysConsiderOld)
)

// flakyIssue is a wrapper of github.Issue, used for storing pre-computed information
type flakyIssue struct {
	issue    *github.Issue
	identity string               // identity discovered by matching reTestIdentifier
	comment  *github.IssueComment // The first auto comment, updated for every history
}

// getIdentityForTest creates a unique string for a test, which will be used for identifying Github issue
func getIdentityForTest(testFullName, repoName string) string {
	return fmt.Sprintf("'%s' in repo '%s'", testFullName, repoName)
}

// getBulkIssueIdentity creates a unique identity for a bulk issue when the flaky rate is above a threshold
func getBulkIssueIdentity(rd RepoData, flakyRate float32) string {
	return fmt.Sprintf("%.2f%% tests failed in repo %s on %s",
		flakyRate*100, rd.Config.Repo, time.Unix(*rd.LastBuildStartTime, 0).String())
}

// GithubIssueHandler handles methods for github issues
type GithubIssueHandler struct {
	user   *github.User
	client ghutil.GithubOperations
}

// Setup creates the necessary setup to make calls to work with github issues
func Setup(githubToken string) (*GithubIssueHandler, error) {
	ghc, err := ghutil.NewGithubClient(githubToken)
	if err != nil {
		return nil, fmt.Errorf("Cannot authenticate to github: %v", err)
	}

	ghUser, err := ghc.GetGithubUser()
	if err != nil {
		return nil, fmt.Errorf("Cannot get username: %v", err)
	}
	return &GithubIssueHandler{user: ghUser, client: ghc}, nil
}

// The Repo field of an github Issue could be empty, use URL is more reliable
func getRepoFromIssue(issue *github.Issue) string {
	_, repo := path.Split(*(issue.RepositoryURL))
	return repo
}

// createCommentForTest summarizes latest status of current test case,
// and creates text to be added to issue comment
func (gih *GithubIssueHandler) createCommentForTest(rd RepoData, testFullName string) string {
	ts := rd.TestStats[testFullName]
	totalCount := len(ts.Passed) + len(ts.Skipped) + len(ts.Failed)
	lastBuildStartTimeStr := time.Unix(*rd.LastBuildStartTime, 0).String()
	content := fmt.Sprintf("%s\nLast build start time: %s\nFailed %d times out of %d runs.",
		fmt.Sprintf(latestStatusPattern, ts.getTestStatus()),
		lastBuildStartTimeStr, len(ts.Failed), totalCount)
	if len(ts.Failed) > 0 {
		content += " Failed runs: "
		var buildIDContents []string
		for _, buildID := range ts.Failed {
			buildIDContents = append(buildIDContents,
				fmt.Sprintf("[%d](%s%s/%d)", buildID, jobLogsURL, rd.Config.Name, buildID))
		}
		content += strings.Join(buildIDContents, ", ")
	}
	return content
}

// create unicode graphs for current scan as well as all previous scans
func (gih *GithubIssueHandler) createHistoryUnicode(rd RepoData, comment, testFullName string) string {
	currentUnicode := fmt.Sprintf("%s: ", time.Unix(*rd.LastBuildStartTime, 0).String())
	resultSlice := rd.getResultSliceForTest(testFullName)
	for i, buildID := range rd.BuildIDs {
		url := fmt.Sprintf("%s%s/%d", jobLogsURL, rd.Config.Name, buildID)
		var statusUnicode string
		switch resultSlice[i] {
		case junit.Passed:
			statusUnicode = passedUnicode
		case junit.Failed:
			statusUnicode = failedUnicode
		default:
			statusUnicode = skippedUnicode
		}
		currentUnicode += fmt.Sprintf(" [%s](%s)", statusUnicode, url)
	}

	// Make sure there is no dupe of records
	uniqHistoryEntries := sets.String{}
	oldHistory := reSingleRecordRegex.FindAllStringSubmatch(comment, -1)
	for _, hist := range oldHistory {
		// Remove <!------End of History------>" that were introduced on some lines.
		// TODO(chaodaiG): removing this logic once this is all cleared in
		// existing bugs.
		uniqHistoryEntries.Insert(strings.ReplaceAll(hist[0], afterHistoryToken, ""))
	}
	records := append([]string{currentUnicode}, uniqHistoryEntries.List()...)
	sort.Slice(records, func(i, j int) bool {
		return records[i] > records[j]
	})
	// There is a 65535 characters limit for Github comment. As tested
	// (https://github.com/chaodaiG/test-github-api/issues/129#issuecomment-527972076),
	// one comment can at least contain 120 records, keep only 60 records to be
	// safe. See https://github.com/knative/test-infra/issues/1326
	if len(records) > 60 {
		records = records[:60]
	}

	res := fmt.Sprintf("\n%s\n", strings.Join(records, "\n"))
	if len(records) > maxHistoryEntries {
		res = fmt.Sprintf("\n%s\n%s\n", strings.Join(records[:maxHistoryEntries], "\n"),
			fmt.Sprintf(collapseTemplate, "Older builds", strings.Join(records[maxHistoryEntries:], "\n")))
	}

	return fmt.Sprintf(historyPattern, res)
}

// updateIssue adds comments to an existing issue, close an issue if test passed both in previous day and today,
// reopens the issue if test becomes flaky while issue is closed.
func (gih *GithubIssueHandler) updateIssue(fi flakyIssue, newComment string, ts *TestStat, dryrun bool) error {
	issue := fi.issue
	passedLastTime := false
	latestStatus := reLatestStatusRegex.FindStringSubmatch(fi.comment.GetBody())
	if len(latestStatus) >= 2 {
		switch latestStatus[1] {
		case passedStatus:
			passedLastTime = true
		case flakyStatus, failedStatus, lackDataStatus:
			// for now no action is needed
		default:
			return fmt.Errorf("invalid test status code found from issue '%s'", *issue.URL)
		}
	}

	// Update comment unless test passed and issue closed
	if !ts.isPassed() || issue.GetState() == string(ghutil.IssueOpenState) {
		if err := helpers.Run(
			"updating comment",
			func() error {
				return gih.client.EditComment(org, getRepoFromIssue(issue), *fi.comment.ID, newComment)
			},
			dryrun); err != nil {
			return fmt.Errorf("failed updating comments for issue '%s': '%v'", *issue.URL, err)
		}
	}

	if ts.isPassed() { // close open issue if the test passed twice consecutively
		if issue.GetState() == string(ghutil.IssueOpenState) && passedLastTime {
			if err := helpers.Run(
				"closing issue",
				func() error {
					closeErr := gih.client.CloseIssue(org, getRepoFromIssue(issue), *issue.Number)
					if closeErr == nil {
						closeComment := "Closing issue: this test has passed in latest 2 scans"
						_, closeErr = gih.client.CreateComment(org, getRepoFromIssue(issue), *issue.Number, closeComment)
					}
					return closeErr
				},
				dryrun); err != nil {
				return fmt.Errorf("failed closing issue '%s': '%v'", *issue.URL, err)
			}
		}
	} else if ts.isFlaky() { // reopen closed issue if test found flaky
		if issue.GetState() == string(ghutil.IssueCloseState) {
			if err := helpers.Run(
				"reopening issue",
				func() error {
					openErr := gih.client.ReopenIssue(org, getRepoFromIssue(issue), *issue.Number)
					if openErr == nil {
						openComment := "Reopening issue: this test is flaky"
						_, openErr = gih.client.CreateComment(org, getRepoFromIssue(issue), *issue.Number, openComment)
					}
					return openErr
				},
				dryrun); err != nil {
				return fmt.Errorf("failed reopen issue: '%s'", *issue.URL)
			}
		}
	}
	return nil
}

// createNewIssue creates an issue, adds flaky label and adds comment.
func (gih *GithubIssueHandler) createNewIssue(org, repoForIssue, title, body, comment string, dryrun bool) (*github.Issue, error) {
	var newIssue *github.Issue
	if err := helpers.Run(
		"creating issue",
		func() error {
			var err error
			newIssue, err = gih.client.CreateIssue(org, repoForIssue, title, body)
			return err
		},
		dryrun); err != nil {
		return nil, fmt.Errorf("failed creating issue '%s' in repo '%s'", title, repoForIssue)
	}
	var addIdentityErrs []error // clean up issue if any error occurred during adding identity, see below
	if err := helpers.Run(
		"adding comment",
		func() error {
			_, err := gih.client.CreateComment(org, repoForIssue, *newIssue.Number, comment)
			return err
		},
		dryrun); err != nil {
		addIdentityErrs = append(addIdentityErrs, fmt.Errorf("failed adding comment to issue '%s', '%v'", *newIssue.URL, err))
	}
	if helpers.CombineErrors(addIdentityErrs) == nil {
		if err := helpers.Run(
			"adding flaky label",
			func() error {
				return gih.client.AddLabelsToIssue(org, repoForIssue, *newIssue.Number, []string{flakyLabel})
			},
			dryrun); err != nil {
			addIdentityErrs = append(addIdentityErrs, fmt.Errorf("failed adding '%s' label to issue '%s', '%v'", flakyLabel, *newIssue.URL, err))
		}
	}
	// This tool is designed to ensure a very small pool of issues related to flaky tests, by minimizing
	// chances of duplicate issues. If for any reason the created issue failed to be labeled with correct identities,
	// this issue will be invalid, and it's very likely that the same issue will be created the next time around.
	// So cleanup issue if failed adding identity, by removing flaky label and closing issue
	if helpers.CombineErrors(addIdentityErrs) != nil {
		if err := helpers.Run(
			"cleaning up invalid issue",
			func() error {
				var gErr []error
				if rlErr := gih.client.RemoveLabelForIssue(org, repoForIssue, *newIssue.Number, flakyLabel); rlErr != nil {
					gErr = append(gErr, rlErr)
				}
				if cErr := gih.client.CloseIssue(org, repoForIssue, *newIssue.Number); cErr != nil {
					gErr = append(gErr, cErr)
				}
				return helpers.CombineErrors(gErr)
			},
			dryrun); err != nil {
			addIdentityErrs = append(addIdentityErrs, err)
		}
	}
	return newIssue, helpers.CombineErrors(addIdentityErrs)
}

// findExistingComment identify existing comment by comment author and test identifier,
// if multiple comments were found return the earliest one.
func (gih *GithubIssueHandler) findExistingComment(issue *github.Issue, issueIdentity string) (*github.IssueComment, error) {
	var targetComment *github.IssueComment
	comments, err := gih.client.ListComments(org, getRepoFromIssue(issue), *issue.Number)
	if err != nil {
		return nil, err
	}
	sort.Slice(comments, func(i, j int) bool {
		return comments[i].CreatedAt != nil && (comments[j].CreatedAt == nil || comments[i].CreatedAt.Before(*comments[j].CreatedAt))
	})

	for i, comment := range comments {
		if *comment.User.ID != *gih.user.ID {
			continue
		}
		// Double check to make sure the comment contains beforeHistoryToken as
		// it's expected from auto-comment. Check reTestIdentifierRegex for bulk
		// issue since it doesn't have beforeHistoryToken
		testNameFromComment := reTestIdentifierRegex.FindStringSubmatch(*comment.Body)
		if (len(testNameFromComment) >= 2 && issueIdentity == testNameFromComment[1]) ||
			strings.Contains(*comment.Body, beforeHistoryToken) {
			targetComment = comments[i]
			break
		}
	}
	if targetComment == nil {
		return nil, fmt.Errorf("no comment match")
	}
	return targetComment, nil
}

// githubToFlakyIssue converts a github issue into a flakyIssue struct
func (gih *GithubIssueHandler) githubToFlakyIssue(issue *github.Issue, dryrun bool) (*flakyIssue, error) {
	// Issues are not created when using dryrun, so skip them
	if dryrun {
		return nil, nil
	}
	// Issue closed long time ago, it might fail with a different reason now.
	if issue.ClosedAt != nil && issue.ClosedAt.Before(timeConsiderOld) {
		return nil, nil
	}

	issueID := reTestIdentifierRegex.FindStringSubmatch(issue.GetBody())
	// Malformed issue, all auto flaky issues need to be identifiable.
	if len(issueID) < 2 {
		return nil, fmt.Errorf("test identifier '%s' is malformed", issueID)
	}
	autoComment, err := gih.findExistingComment(issue, issueID[1])
	if err != nil {
		return nil, fmt.Errorf("cannot find auto comment for issue '%s': '%v'", *issue.URL, err)
	}

	return &flakyIssue{
		issue:    issue,
		identity: issueID[1],
		comment:  autoComment,
	}, nil
}

// getFlakyIssues filters all issues by flakyLabel, and return map {testName: slice of issues}
// Fail if find any issue with no discoverable identifier(testIdentifierPattern missing),
// also fail if auto comment not found.
// In most cases there is only 1 issue for each testName, if multiple issues found open for same test,
// most likely it's caused by old issues being reopened manually, in this case update both issues.
func (gih *GithubIssueHandler) getFlakyIssues() (map[string][]flakyIssue, error) {
	issuesMap := make(map[string][]flakyIssue)
	reposForIssue, err := gih.client.ListRepos(org)
	if err != nil {
		return nil, err
	}
	for _, repoForIssue := range reposForIssue {
		issues, err := gih.client.ListIssuesByRepo(org, repoForIssue, []string{flakyLabel})
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
			fi, err := gih.githubToFlakyIssue(issue, false)
			if err != nil {
				return nil, err
			}
			if fi != nil {
				issuesMap[fi.identity] = append(issuesMap[fi.identity], *fi)
			}
		}
	}
	// Handle test with multiple issues associated
	// if all open: update all of them
	// if all closed: only keep latest one
	// otherwise(these may have been closed manually): remove closed ones from map
	for k, v := range issuesMap {
		var hasOpen, hasClosed bool
		for _, fi := range v {
			switch fi.issue.GetState() {
			case string(ghutil.IssueOpenState):
				hasOpen = true
			case string(ghutil.IssueCloseState):
				hasClosed = true
			}
		}
		if hasOpen && hasClosed {
			for i, fi := range v {
				if string(ghutil.IssueCloseState) == fi.issue.GetState() {
					issuesMap[k] = append(issuesMap[k][:i], issuesMap[k][i+1:]...)
				}
			}
		} else if !hasOpen {
			sort.Slice(issuesMap[k], func(i, j int) bool {
				return issuesMap[k][i].issue.CreatedAt != nil &&
					(issuesMap[k][j].issue.CreatedAt == nil || issuesMap[k][i].issue.CreatedAt.Before(*issuesMap[k][j].issue.CreatedAt))
			})
			issuesMap[k] = []flakyIssue{issuesMap[k][0]}
		}
	}
	return issuesMap, err
}

// processGithubIssuesForRepo reads RepoData and existing issues, and create/close/reopen/comment on issues.
// The function returns:
// Slice of newly created Github issues, if any
// Slice of messages containing performed actions,
// Slice of error messages.
func (gih *GithubIssueHandler) processGithubIssuesForRepo(rd RepoData, flakyIssuesMap map[string][]flakyIssue, dryrun bool) ([]flakyIssue, []string, error) {
	if len(rd.Config.IssueRepo) == 0 {
		return nil, []string{"skip creating/updating issues, job is marked to not create GitHub issues\n"}, nil
	}

	// If there are too many failures, create a single issue tracking it.
	if flakyRateAboveThreshold(rd) {
		flakyRate := getFlakyRate(rd)
		log.Printf("flaky rate above threshold, creating a single issue")
		identity := getBulkIssueIdentity(rd, flakyRate)
		if _, ok := flakyIssuesMap[identity]; ok {
			log.Printf("issue already exist, skip creating")
			return nil, nil, nil
		}

		testId := fmt.Sprintf(testIdentifierPattern, identity)
		message := fmt.Sprintf("Creating issue '%s' in repo '%s'", identity, rd.Config.IssueRepo)
		log.Println(message)
		issue, err := gih.createNewIssue(
			org,
			rd.Config.IssueRepo,
			fmt.Sprintf("[flaky] %s", identity),
			fmt.Sprintf(issueBodyTemplate, identity, rd.Config.Repo, testId),
			fmt.Sprintf("Bulk issue tracking: %s\n<!--%s-->", identity, testId),
			dryrun,
		)
		if err != nil {
			return nil, []string{message}, err
		}

		fi, err := gih.githubToFlakyIssue(issue, dryrun)
		if err != nil || fi == nil {
			return []flakyIssue{}, []string{message}, err
		}
		return []flakyIssue{*fi}, []string{message}, nil
	}

	var (
		messages []string
		errs     []error
		issues   []flakyIssue
	)

	// Update/Create issues for flaky/used-to-be-flaky tests
	for testFullName, ts := range rd.TestStats {
		if !ts.isFlaky() && !ts.isPassed() {
			continue
		}
		identity := getIdentityForTest(testFullName, rd.Config.Repo)
		comment := gih.createCommentForTest(rd, testFullName)
		if existIssues, ok := flakyIssuesMap[identity]; ok { // update issue with current result
			for _, existIssue := range existIssues {
				if strings.Contains(existIssue.comment.GetBody(), comment) {
					log.Printf("skip updating issue '%s', as it already contains data for run '%d'\n",
						*existIssue.issue.URL, *rd.LastBuildStartTime)
					continue
				}
				comment += gih.createHistoryUnicode(rd, existIssue.comment.GetBody(), testFullName)
				message := fmt.Sprintf("Updating issue '%s' for '%s'", *existIssue.issue.URL, existIssue.identity)
				log.Println(message)
				messages = append(messages, message)
				if err := gih.updateIssue(existIssue, comment, ts, dryrun); err != nil {
					log.Println(err)
					errs = append(errs, err)
				}
			}
		} else if ts.isFlaky() {
			comment = fmt.Sprintf("%s%s\n<!--%s-->", comment, gih.createHistoryUnicode(rd, "", testFullName),
				fmt.Sprintf(testIdentifierPattern, identity))
			message := fmt.Sprintf("Creating issue '%s' in repo '%s'", testFullName, rd.Config.IssueRepo)
			log.Println(message)
			messages = append(messages, message)
			if issue, err := gih.createNewIssue(
				org,
				rd.Config.IssueRepo,
				fmt.Sprintf("[flaky] %s", testFullName),
				fmt.Sprintf(issueBodyTemplate, testFullName, rd.Config.Repo, fmt.Sprintf(testIdentifierPattern, identity)),
				comment,
				dryrun); err != nil {
				log.Println(err)
				errs = append(errs, err)
			} else {
				if fi, err := gih.githubToFlakyIssue(issue, dryrun); err != nil {
					errs = append(errs, err)
				} else if !dryrun { // fi is nil as issue is not created in dryrun mode
					issues = append(issues, *fi)
				}
			}
		}
	}
	return issues, messages, helpers.CombineErrors(errs)
}

// analyze all results, figure out flaky tests and processing existing auto:flaky issues
func (gih *GithubIssueHandler) processGithubIssues(repoDataAll []RepoData, dryrun bool) (map[string][]flakyIssue, error) {
	// Collect all flaky test issues from all knative repos, in case issues are moved around
	// Fail this job if data collection failed
	flakyGHIssuesMap, err := gih.getFlakyIssues()
	if err != nil {
		log.Fatalf("%v", err)
	}

	// map repo to jobs, and jobs to messages
	messagesMap := make(map[string]map[string][]string)
	// map repo to jobs, and jobs to errors
	errMap := make(map[string]map[string][]error)

	for _, rd := range repoDataAll {
		issues, messages, err := gih.processGithubIssuesForRepo(rd, flakyGHIssuesMap, dryrun)
		if _, ok := messagesMap[rd.Config.Repo]; !ok {
			messagesMap[rd.Config.Repo] = make(map[string][]string)
		}
		messagesMap[rd.Config.Repo][rd.Config.Name] = messages
		flakyGHIssuesMap[rd.Config.Repo] = append(flakyGHIssuesMap[rd.Config.Repo], issues...)

		if err != nil {
			if _, ok := errMap[rd.Config.Repo]; !ok {
				errMap[rd.Config.Repo] = make(map[string][]error)
			}
			errMap[rd.Config.Repo][rd.Config.Name] = append(errMap[rd.Config.Repo][rd.Config.Name], err)
		}
	}

	gih.logSummary(repoDataAll, messagesMap, errMap)

	return flakyGHIssuesMap, nil
}

// logSummary will log the overall summary for all repos and all jobs
// repoDataAll => Information about all the repos and jobs
// messagesMap => { RepoName -> { JobName -> []Message }}
// errMap => { RepoName -> { JobName -> []errors }}
func (gih *GithubIssueHandler) logSummary(repoDataAll []RepoData, messagesMap map[string]map[string][]string, errMap map[string]map[string][]error) {
	summary := "Summary:\n"
	for _, rd := range repoDataAll {
		if messages, ok := messagesMap[rd.Config.Repo][rd.Config.Name]; ok {
			summary += fmt.Sprintf("Summary of job '%s' in repo '%s':\n", rd.Config.Name, rd.Config.Repo)
			summary += strings.Join(messages, "\n")
		}
		if errs, ok := errMap[rd.Config.Repo][rd.Config.Name]; ok {
			summary += fmt.Sprintf("Errors in job '%s' in repo '%s':\n%v", rd.Config.Name, rd.Config.Repo, helpers.CombineErrors(errs))
		}
	}

	log.Println(summary)
}
