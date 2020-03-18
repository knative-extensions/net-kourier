/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package githubFakes

import (
	"context"
	"log"
	"path"

	"github.com/google/go-github/github"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubClient"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubPr"
	"knative.dev/test-infra/tools/coverage/test"
)

func FakeGithubClient() *githubClient.GithubClient {
	return githubClient.New(fakeGithubIssues(), fakePullRequests())
}

func testCommitFile(filename string) *github.CommitFile {
	filename = path.Join(test.CovTargetRelPath, filename)
	return &github.CommitFile{
		Filename: &filename,
	}
}

func testCommitFiles() (res []*github.CommitFile) {
	return []*github.CommitFile{
		testCommitFile("onlySrcChange.go"),
		testCommitFile("onlyTestChange_test.go"),
		testCommitFile("common.go"),
		testCommitFile("cov-excl.go"),
		testCommitFile("ling-gen_test.go"),
		testCommitFile("newlyAddedFile.go"),
		testCommitFile("newlyAddedFile_test.go"),
	}
}

type FakeGithubIssues struct {
	githubClient.Issues
}

type FakeGithubPullRequests struct {
	githubClient.PullRequests
}

func fakeGithubIssues() githubClient.Issues {
	return &FakeGithubIssues{}
}

func fakePullRequests() githubClient.PullRequests {
	return &FakeGithubPullRequests{}
}

func (issues *FakeGithubIssues) CreateComment(ctx context.Context, owner string, repo string,
	number int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	log.Printf("FakeGithubIssues.CreateComment(Ctx, owner=%s, repo=%s, number=%d, "+
		"comment.GetBody()=%s) called\n", owner, repo, number, comment.GetBody())
	return nil, nil, nil
}

func (issues *FakeGithubIssues) DeleteComment(ctx context.Context, owner string, repo string,
	commentID int64) (*github.Response, error) {
	log.Printf("FakeGithubIssues.DeleteComment(Ctx, owner=%s, repo=%s, commentID=%d) called\n",
		owner, repo, commentID)
	return nil, nil
}

func (issues *FakeGithubIssues) ListComments(ctx context.Context, owner string, repo string, number int,
	opt *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error) {
	log.Printf("FakeGithubIssues.ListComment(Ctx, owner=%s, repo=%s, number=%d, "+
		"opt=%v) called\n", owner, repo, number, opt)
	return nil, nil, nil
}

func (pr *FakeGithubPullRequests) ListFiles(ctx context.Context, owner string, repo string, number int, opt *github.ListOptions) (
	[]*github.CommitFile, *github.Response, error) {
	if opt == nil {
		return testCommitFiles(), nil, nil
	}
	files := testCommitFiles()
	rsp := &github.Response{
		NextPage: (opt.Page + 1) % len(files),
	}
	return []*github.CommitFile{files[opt.Page]}, rsp, nil

}

func FakeRepoData() *githubPr.GithubPr {
	ctx := context.Background()
	log.Printf("creating fake repo data \n")

	return &githubPr.GithubPr{
		RepoOwner:     "fakeRepoOwner",
		RepoName:      "fakeRepoName",
		Pr:            7,
		RobotUserName: "fakeCovbot",
		GithubClient:  FakeGithubClient(),
		Ctx:           ctx,
	}
}
