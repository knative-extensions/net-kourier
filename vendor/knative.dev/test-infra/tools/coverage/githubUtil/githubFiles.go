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
package githubUtil

import (
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/google/go-github/github"
	"knative.dev/test-infra/tools/coverage/git"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubPr"
	"knative.dev/test-infra/tools/coverage/logUtil"
)

// return corresponding source file path of given path (abc_test.go -> abc.go)
func sourceFilePath(path string) string {
	if strings.HasSuffix(path, "_test.go") {
		return strings.TrimSuffix(path, "_test.go") + ".go"
	}
	return path
}

// Get the list of files in a commit, excluding those to be ignored by coverage
func GetConcernedFiles(data *githubPr.GithubPr, filePathPrefix string) map[string]bool {
	listOptions := &github.ListOptions{Page: 1}

	fmt.Println()
	log.Printf("GetConcernedFiles(...) started\n")

	commitFiles := make([]*github.CommitFile, 0)
	for {
		files, rsp, err := data.GithubClient.PullRequests.ListFiles(data.Ctx, data.RepoOwner, data.RepoName,
			data.Pr, listOptions)
		if err != nil {
			logUtil.LogFatalf("error running c.PullRequests.ListFiles("+
				"Ctx, repoOwner=%s, RepoName=%s, pullNum=%v, listOptions): %v\n",
				data.RepoOwner, data.RepoName, data.Pr, err)
			return nil
		}
		commitFiles = append(commitFiles, files...)
		if rsp.NextPage == 0 {
			break
		}
		listOptions.Page = rsp.NextPage
	}

	fileNames := make(map[string]bool)
	for i, commitFile := range commitFiles {
		filePath := path.Join(filePathPrefix, sourceFilePath(*commitFile.Filename))
		isFileConcerned := !git.IsCoverageSkipped(filePath)
		log.Printf("github file #%d: %s, concerned=%v\n", i, filePath, isFileConcerned)
		fileNames[filePath] = isFileConcerned
	}

	log.Printf("GetConcernedFiles(...) completed\n\n")
	return fileNames
}
