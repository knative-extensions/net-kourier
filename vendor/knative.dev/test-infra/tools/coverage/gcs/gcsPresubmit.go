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

/*
Package main prototypes uploading resource (go test coverage profile) to GCS
if enable debug, then the reading from GCS feature would be run as well
*/

package gcs

import (
	"path"
	"strconv"

	"knative.dev/test-infra/tools/coverage/artifacts"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubPr"
)

const (
	ArtifactsDirNameOnGcs = "artifacts"
	gcsUrlHost            = "storage.cloud.google.com/"
)

type PresubmitBuild struct {
	GcsBuild
	Artifacts     GcsArtifacts
	PostSubmitJob string
}

type PreSubmit struct {
	githubPr.GithubPr
	PresubmitBuild
}

func (p *PreSubmit) relDirOfArtifacts() (result string) {
	dir := path.Join("pr-logs", "pull", p.RepoOwner+"_"+p.RepoName, p.PrStr(), p.Job)
	return path.Join(path.Join(dir, strconv.Itoa(p.Build)), ArtifactsDirNameOnGcs)
}

func (p *PreSubmit) MakeGcsArtifacts(localArts artifacts.LocalArtifacts) *GcsArtifacts {
	localArts.SetDirectory(p.relDirOfArtifacts())
	return NewGcsArtifacts(p.Ctx, p.Client, p.Bucket, localArts.Artifacts)
}

func (p *PreSubmit) urlLineCov() (result string) {
	dir := path.Join(gcsUrlHost, p.Bucket, p.relDirOfArtifacts())
	return path.Join(dir, artifacts.LineCovFileName)
}

func (p *PreSubmit) UrlGcsLineCovLinkWithMarker(section int) (result string) {
	return "https://" + p.urlLineCov() + "#file" + strconv.Itoa(section)
}
