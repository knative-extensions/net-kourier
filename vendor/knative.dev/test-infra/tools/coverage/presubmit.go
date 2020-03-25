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

package main

import (
	"log"

	"knative.dev/test-infra/tools/coverage/artifacts"
	"knative.dev/test-infra/tools/coverage/calc"
	"knative.dev/test-infra/tools/coverage/gcs"
	"knative.dev/test-infra/tools/coverage/githubUtil"
	"knative.dev/test-infra/tools/coverage/io"
	"knative.dev/test-infra/tools/coverage/line"
)

// RunPresubmit runs the pre-submit procedure
func RunPresubmit(p *gcs.PreSubmit, arts *artifacts.LocalArtifacts) (bool, error) {
	log.Println("starting PreSubmit.RunPresubmit(...)")

	// concerned files is a collection of all the files whose coverage change will be reported
	var concernedFiles map[string]bool

	if p.GithubClient != nil {
		concernedFiles = githubUtil.GetConcernedFiles(&p.GithubPr, "")
		if len(concernedFiles) == 0 {
			log.Printf("List of concerned committed files is empty, " +
				"don't need to run coverage profile in presubmit\n")
			return false, nil
		}
	}

	gNew := calc.CovList(arts.ProfileReader(), arts.KeyProfileCreator(),
		concernedFiles, p.CovThreshold)
	err := line.CreateLineCovFile(arts)
	line.GenerateLineCovLinks(p, gNew)

	base := gcs.NewPostSubmit(p.Ctx, p.Client, p.Bucket,
		p.PostSubmitJob, gcs.ArtifactsDirNameOnGcs, arts.ProfileName())
	gBase := calc.CovList(base.ProfileReader(), nil, concernedFiles, p.CovThreshold)
	changes := calc.NewGroupChanges(gBase, gNew)

	postContent, isEmpty, isCoverageLow := changes.ContentForGithubPost(concernedFiles)

	io.Write(&postContent, arts.Directory(), "bot-post")

	if !isEmpty && p.GithubClient != nil {
		err = p.GithubPr.CleanAndPostComment(postContent)
	}

	log.Println("completed PreSubmit.RunPresubmit(...)")
	return isCoverageLow, err
}
