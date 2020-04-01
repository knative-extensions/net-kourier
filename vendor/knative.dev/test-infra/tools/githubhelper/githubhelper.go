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
	"log"
	"os"
	"strconv"

	"github.com/google/go-github/github"
	"knative.dev/pkg/test/ghutil"
)

var (
	// Info about the current PR
	repoOwner  = os.Getenv("REPO_OWNER")
	repoName   = os.Getenv("REPO_NAME")
	pullNumber = atoi(os.Getenv("PULL_NUMBER"), "pull number")

	// Shared useful variables
	ctx     = context.Background()
	verbose = false
	client  *ghutil.GithubClient
)

// authenticate creates client with given token if it's provided and exists,
// otherwise it falls back to use an anonymous client
func authenticate(githubTokenPath *string) {
	var err error
	client, err = ghutil.NewGithubClient(*githubTokenPath)
	if err != nil {
		infof("Error creating client with token %q: %v", *githubTokenPath, err)
		infof("Proceeding with unauthenticated client")
		client = &ghutil.GithubClient{Client: github.NewClient(nil)}
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
	files, err := client.ListFiles(repoOwner, repoName, pullNumber)
	if err != nil {
		log.Fatalf("Error listing files: %v", err)
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
