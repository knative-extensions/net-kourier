/*
Copyright 2018 The Knative Authors

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

// githubhelper.go interacts with GitHub, providing useful data for a Prow job.

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var (
	// Info about the current PR
	repoOwner  = os.Getenv("REPO_OWNER")
	repoName   = os.Getenv("REPO_NAME")
	pullNumber = atoi(os.Getenv("PULL_NUMBER"), "pull number")

	// Shared useful variables
	ctx     = context.Background()
	verbose = false
	client  *github.Client
)

// authenticate creates client with given token if it's provided and exists,
// otherwise it falls back to use an anonymous client
func authenticate(githubTokenPath *string) {
	client = github.NewClient(nil)
	if githubTokenPath == nil || *githubTokenPath == "" {
		infof("Using unauthenticated github client")
		return
	}

	infof("Reading github token from file %q", *githubTokenPath)
	if b, err := ioutil.ReadFile(*githubTokenPath); err == nil {
		infof("Authenticating using provided github token")
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: strings.TrimSpace(string(b))},
		)
		client = github.NewClient(oauth2.NewClient(ctx, ts))
	} else {
		infof("Error reading file %q: %v", *githubTokenPath, err)
		infof("Proceeding unauthenticated")
	}
}

// atoi is a convenience function to convert a string to integer, failing in case of error.
func atoi(str, valueName string) int {
	value, err := strconv.Atoi(str)
	if err != nil {
		log.Fatalf("Unexpected non number '%s' for %s: %v", str, valueName, err)
	}
	return value
}

// infof if a convenience wrapper around log.Infof, and does nothing unless --verbose is passed.
func infof(template string, args ...interface{}) {
	if verbose {
		log.Printf(template, args...)
	}
}

// listChangedFiles simply lists the files changed by the current PR.
func listChangedFiles() {
	infof("Listing changed files for PR %d in repository %s/%s", pullNumber, repoOwner, repoName)
	files := make([]*github.CommitFile, 0)

	listOptions := &github.ListOptions{
		Page:    1,
		PerPage: 300,
	}
	for {
		commitFiles, rsp, err := client.PullRequests.ListFiles(ctx, repoOwner, repoName, pullNumber, listOptions)
		if err != nil {
			log.Fatalf("Error listing files: %v", err)
		}
		files = append(files, commitFiles...)
		if rsp.NextPage == 0 {
			break
		}
		listOptions.Page = rsp.NextPage
	}

	for _, file := range files {
		fmt.Println(*file.Filename)
	}
}

func main() {
	githubTokenPath := flag.String("github-token", os.Getenv("GITHUB_BOT_TOKEN"), "Github token file path for authenticating with Github")
	listChangedFilesFlag := flag.Bool("list-changed-files", false, "List the files changed by the current pull request")
	verboseFlag := flag.Bool("verbose", false, "Whether to dump extra info on output or not; intended for debugging")
	flag.Parse()

	verbose = *verboseFlag
	authenticate(githubTokenPath)

	if *listChangedFilesFlag {
		listChangedFiles()
	}
}
