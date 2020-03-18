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
	"context"
	"fmt"
	"testing"

	"knative.dev/test-infra/tools/coverage/artifacts/artsTest"
	"knative.dev/test-infra/tools/coverage/gcs"
	"knative.dev/test-infra/tools/coverage/gcs/gcsFakes"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubClient"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubFakes"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubPr"
	"knative.dev/test-infra/tools/coverage/test"
)

const (
	testPresubmitBuild = 787
)

func TestRunPresubmit(t *testing.T) {
	tests := []struct {
		name     string
		ghClient *githubClient.GithubClient
		want     bool
		wantErr  bool
	}{
		{
			"RunPresubmitWithFakeGithubClient",
			githubFakes.FakeGithubClient(),
			false,
			false,
		},
		{
			"RunPresubmitWithNoGithubClient",
			nil,
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoData := &githubPr.GithubPr{
				RepoOwner:     "fakeRepoOwner",
				RepoName:      "fakeRepoName",
				Pr:            7,
				RobotUserName: "fakeCovbot",
				GithubClient:  tt.ghClient,
				Ctx:           context.Background(),
			}

			pbuild := gcs.PresubmitBuild{
				GcsBuild: gcs.GcsBuild{
					Client: gcsFakes.NewFakeStorageClient(),
					Bucket: gcsFakes.FakeGcsBucketName,
					Job:    gcsFakes.FakePreSubmitProwJobName,
					Build:  testPresubmitBuild,
				},
				Artifacts: gcs.GcsArtifacts{
					Ctx:       context.Background(),
					Bucket:    "fakeBucket",
					Client:    gcsFakes.NewFakeStorageClient(),
					Artifacts: artsTest.LocalArtsForTest("gcsArts-").Artifacts,
				},
				PostSubmitJob: gcsFakes.FakePostSubmitProwJobName,
			}

			preSubmit := &gcs.PreSubmit{
				GithubPr:       *repoData,
				PresubmitBuild: pbuild,
			}

			arts := artsTest.LocalArtsForTest("TestRunPresubmit")
			arts.ProduceProfileFile("./" + test.CovTargetRelPath)

			got, err := RunPresubmit(preSubmit, arts)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunPresubmit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RunPresubmit() = %v, want %v", got, tt.want)
			}
		})
	}
}

// tests the construction of gcs url from PreSubmit
func TestK8sGcsAddress(t *testing.T) {
	repoData := &githubPr.GithubPr{
		RepoOwner:     "fakeRepoOwner",
		RepoName:      "fakeRepoName",
		Pr:            7,
		RobotUserName: "fakeCovbot",
		GithubClient:  githubFakes.FakeGithubClient(),
		Ctx:           context.Background(),
	}

	pbuild := gcs.PresubmitBuild{
		GcsBuild: gcs.GcsBuild{
			Client: gcsFakes.NewFakeStorageClient(),
			Bucket: gcsFakes.FakeGcsBucketName,
			Job:    gcsFakes.FakePreSubmitProwJobName,
			Build:  testPresubmitBuild,
		},
		Artifacts: gcs.GcsArtifacts{
			Ctx:       context.Background(),
			Bucket:    "fakeBucket",
			Client:    gcsFakes.NewFakeStorageClient(),
			Artifacts: artsTest.LocalArtsForTest("gcsArts-").Artifacts,
		},
		PostSubmitJob: gcsFakes.FakePostSubmitProwJobName,
	}

	presubmitData := &gcs.PreSubmit{
		GithubPr:       *repoData,
		PresubmitBuild: pbuild,
	}
	presubmitData.Build = 1286
	got := presubmitData.UrlGcsLineCovLinkWithMarker(3)

	want := fmt.Sprintf("https://storage.cloud.google.com/%s/pr-logs/pull/"+
		"%s_%s/%s/%s/%s/artifacts/line-cov.html#file3",
		gcsFakes.FakeGcsBucketName, presubmitData.RepoOwner, presubmitData.RepoName, presubmitData.PrStr(), gcsFakes.FakePreSubmitProwJobName, "1286")
	if got != want {
		t.Fatal(test.StrFailure("", want, got))
	}
	fmt.Printf("line cov link=%s", got)
}
