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
package gcs

import (
	"context"
	"log"
	"path"
	"sort"
	"strconv"

	"knative.dev/test-infra/tools/coverage/artifacts"
	"knative.dev/test-infra/tools/coverage/logUtil"
)

type PostSubmit struct {
	GcsBuild
	covProfileName   string
	ArtifactsDirName string
	BuildsSorted     []int
	Ctx              context.Context
}

func NewPostSubmit(ctx context.Context, client StorageClient,
	bucket, prowJobName, artifactsDirName, covProfileName string) (p *PostSubmit) {

	log.Println("NewPostSubmit(Ctx, client StorageClient, ...) started")
	gcsBuild := GcsBuild{
		Client: client,
		Bucket: bucket,
		Build:  -1,
		Job:    prowJobName,
	}
	p = &PostSubmit{
		GcsBuild:         gcsBuild,
		ArtifactsDirName: artifactsDirName,
		covProfileName:   covProfileName,
		Ctx:              ctx,
	}

	p.searchForLatestHealthyBuild()
	return
}

// listBuilds returns all builds in descending order and stores the result in BuildsSorted
func (p *PostSubmit) listBuilds() []int {
	var res []int
	jobDir := path.Join("logs", p.Job)
	lstBuildStrs := p.Client.ListGcsObjects(p.Ctx, p.Bucket, jobDir+"/", "/")
	for _, buildStr := range lstBuildStrs {
		if num, err := strconv.Atoi(buildStr); err != nil {
			log.Printf("None int build number found: '%s'", buildStr)
		} else {
			res = append(res, num)
		}
	}
	if len(res) == 0 {
		logUtil.LogFatalf("No build found for bucket '%s' and object '%s'\n",
			p.Bucket, jobDir)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(res)))
	p.BuildsSorted = res
	log.Printf("Sorted Builds: %v\n", res)
	return res
}

func (p *PostSubmit) dirOfArtifacts(build int) string {
	buildDir := path.Join(path.Join("logs", p.Job), strconv.Itoa(build))
	return path.Join(buildDir, p.ArtifactsDirName)
}

func (p *PostSubmit) isBuildHealthy(build int) bool {
	marker := path.Join(p.dirOfArtifacts(build), artifacts.CovProfileCompletionMarker)
	return p.Client.DoesObjectExist(p.Ctx, p.Bucket, marker)
}

func (p *PostSubmit) pathToGoodCoverageProfile() string {
	return path.Join(p.dirOfArtifacts(p.Build), p.covProfileName)
}

func (p *PostSubmit) searchForLatestHealthyBuild() int {
	builds := p.listBuilds()
	for _, build := range builds {
		if p.isBuildHealthy(build) {
			p.Build = build
			return build
		}
	}
	logUtil.LogFatalf("No healthy build found, builds=%v\n", builds)
	return -1
}

// ProfileReader returns the reader for the most recent healthy profile
func (p *PostSubmit) ProfileReader() *artifacts.ProfileReader {
	profilePath := p.pathToGoodCoverageProfile()
	log.Printf("Reading base (master) coverage from <%s>...\n", profilePath)
	return p.Client.ProfileReader(p.Ctx, p.Bucket, profilePath)
}
