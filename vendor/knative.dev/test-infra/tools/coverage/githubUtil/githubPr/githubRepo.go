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
package githubPr

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"

	"github.com/google/go-github/github"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubClient"
	"knative.dev/test-infra/tools/coverage/logUtil"
)

type GithubPr struct {
	RobotUserName string
	RepoOwner     string
	RepoName      string
	Pr            int
	Ctx           context.Context
	GithubClient  *githubClient.GithubClient
}

func (data *GithubPr) PrStr() string {
	return strconv.Itoa(data.Pr)
}

func New(githubTokenLocation, repoOwner, repoName, prNumStr,
	botUserName string) *GithubPr {
	ctx := context.Background()

	prNum, err := strconv.Atoi(prNumStr)
	if err != nil {
		logUtil.LogFatalf("Failed to convert prNumStr(=%v) to int: %v\n", prNumStr,
			err)
	}

	var client *githubClient.GithubClient
	if githubTokenLocation == "" {
		log.Println("Github token location not provided. Running without github connection")
	} else {
		githubToken, err := getGithubToken(githubTokenLocation)

		if err != nil {
			logUtil.LogFatalf("Failed to get github token: %v\n", err)
		}

		client = githubClient.Make(ctx, githubToken)
	}

	return &GithubPr{
		RepoOwner:     repoOwner,
		RepoName:      repoName,
		Pr:            prNum,
		RobotUserName: botUserName,
		GithubClient:  client,
		Ctx:           ctx}
}

// Create a comment on the repo
func (data *GithubPr) postComment(content string) (err error) {
	log.Printf("client.Issues.CreateComment(Ctx, repoOwner=%s, RepoName=%s, prNum=%v, commentBody)\n",
		data.RepoOwner, data.RepoName, data.Pr)
	commentBody := &github.IssueComment{Body: &content}
	_, _, err = data.GithubClient.Issues.CreateComment(data.Ctx, data.RepoOwner, data.RepoName, data.Pr, commentBody)
	if err != nil {
		log.Printf("error running data.GithubClient.Issues.CreateComment(...):%v\n", err)
	}
	return
}

// Create a comment on the repo
func (data *GithubPr) removeAllBotComments() (nRemoved int, err error) {
	log.Println("removing all bot comments")
	comments, _, err := data.GithubClient.Issues.ListComments(data.Ctx, data.RepoOwner, data.RepoName, data.Pr, nil)

	if err != nil {
		logUtil.LogFatalf("data.GithubClient.Issues.ListComments(..."+
			") returns error: %v\n", err)
	}

	nRemoved = 0
	for _, cmt := range comments {
		userName := *cmt.User.Login
		if userName == data.RobotUserName {
			log.Printf("TO DEL comment: <author=%s> %s\n", userName, *cmt.Body)
			_, err = data.GithubClient.Issues.DeleteComment(
				data.Ctx, data.RepoOwner, data.RepoName, cmt.GetID())

			if err != nil {
				logUtil.LogFatalf("data.GithubClient.Issues.DeleteComment("+
					"data.Ctx, data.RepoOwner, data.RepoName, "+
					"cmt.GetID()) returns error:%v\n", err)
			}

			nRemoved++
		}
	}

	log.Printf(
		"Removed %d comments by robot <%s>", nRemoved, data.RobotUserName)

	return
}

// Remove all existing bot comments, and then create a new comment on the repo
func (data *GithubPr) CleanAndPostComment(content string) (err error) {
	log.Printf("posting on PR *%v*\n", data.Pr)
	_, err = data.removeAllBotComments()
	if err != nil {
		return
	}
	return data.postComment(content)
}

func getGithubToken(githubTokenLocation string) (res string, err error) {
	buf, err := ioutil.ReadFile(githubTokenLocation)
	if err != nil {
		fmt.Printf("github token file cannot be found: %v\n", err)
		return
	}
	res = string(buf)
	if len(res) != 40 {
		fmt.Printf("Warning: token len = %d, which is different from 40\n", len(res))
	}
	return
}
