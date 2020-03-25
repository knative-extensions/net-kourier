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
package githubClient

import (
	"context"
	"log"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type GithubClient struct {
	Issues       Issues
	PullRequests PullRequests
}

func New(issues Issues, pullRequests PullRequests) *GithubClient {
	return &GithubClient{issues, pullRequests}
}

// Get the github client
func Make(ctx context.Context, githubToken string) *GithubClient {
	if len(githubToken) == 0 {
		log.Println("Warning: Github token empty")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return New(client.Issues, client.PullRequests)
}

type Issues interface {
	CreateComment(ctx context.Context, owner string, repo string, number int,
		comment *github.IssueComment) (*github.IssueComment, *github.Response, error)
	DeleteComment(ctx context.Context, owner string, repo string, commentID int64) (
		*github.Response, error)
	ListComments(ctx context.Context, owner string, repo string, number int,
		opt *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error)
}

type PullRequests interface {
	ListFiles(ctx context.Context, owner string, repo string, number int, opt *github.ListOptions) (
		[]*github.CommitFile, *github.Response, error)
}
