/* Copyright 2018 The Knative Authors.

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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"knative.dev/test-infra/tools/coverage/artifacts"
	"knative.dev/test-infra/tools/coverage/gcs"
	"knative.dev/test-infra/tools/coverage/githubUtil/githubPr"
	"knative.dev/test-infra/tools/coverage/logUtil"
	"knative.dev/test-infra/tools/coverage/testgrid"
)

const (
	keyCovProfileFileName      = "key-cov-prof.txt"
	defaultStdoutRedirect      = "stdout.txt"
	defaultCoverageTargetDir   = "."
	defaultGcsBucket           = "knative-prow"
	defaultPostSubmitJobName   = ""
	defaultCovThreshold        = 50
	defaultArtifactsDir        = "./artifacts/"
	defaultCoverageProfileName = "coverage_profile.txt"
)

func main() {
	fmt.Println("entering code coverage main")

	envOverriddenDefaultArtifactsDir := os.Getenv("ARTIFACTS")
	if envOverriddenDefaultArtifactsDir == "" {
		envOverriddenDefaultArtifactsDir = defaultArtifactsDir
	}

	gcsBucketName := flag.String("postsubmit-gcs-bucket", defaultGcsBucket, "gcs bucket name")
	postSubmitJobName := flag.String("postsubmit-job-name", defaultPostSubmitJobName, "name of the prow job")
	artifactsDir := flag.String("artifacts", envOverriddenDefaultArtifactsDir, "directory for artifacts")
	coverageTargetDir := flag.String("cov-target", defaultCoverageTargetDir, "target directory for test coverage")
	coverageProfileName := flag.String("profile-name", defaultCoverageProfileName, "file name for coverage profile")
	githubTokenPath := flag.String("github-token", "", "path to token to access github repo")
	covThreshold := flag.Int("cov-threshold-percentage", defaultCovThreshold, "token to access GitHub repo")
	postingBotUserName := flag.String("posting-robot", "knative-metrics-robot", "github user name for coverage robot")
	flag.Parse()

	log.Printf("container flag list: postsubmit-gcs-bucket=%s; postSubmitJobName=%s; "+
		"artifacts=%s; cov-target=%s; profile-name=%s; github-token=%s; "+
		"cov-threshold-percentage=%d; posting-robot=%s;",
		*gcsBucketName, *postSubmitJobName, *artifactsDir, *coverageTargetDir, *coverageProfileName,
		*githubTokenPath, *covThreshold, *postingBotUserName)

	log.Println("Getting env values")
	pr := os.Getenv("PULL_NUMBER")
	pullSha := os.Getenv("PULL_PULL_SHA")
	baseSha := os.Getenv("PULL_BASE_SHA")
	repoOwner := os.Getenv("REPO_OWNER")
	repoName := os.Getenv("REPO_NAME")
	jobType := os.Getenv("JOB_TYPE")
	jobName := os.Getenv("JOB_NAME")

	log.Printf("Running coverage for PR=%s; PR commit SHA = %s;base SHA = %s", pr, pullSha, baseSha)

	localArtifacts := artifacts.NewLocalArtifacts(
		*artifactsDir,
		*coverageProfileName,
		keyCovProfileFileName,
		defaultStdoutRedirect,
	)

	localArtifacts.ProduceProfileFile(*coverageTargetDir)

	log.Printf("Running workflow: %s\n", jobType)
	switch jobType {
	case "presubmit":
		buildStr := os.Getenv("BUILD_NUMBER")
		build, err := strconv.Atoi(buildStr)
		if err != nil {
			logUtil.LogFatalf("BUILD_NUMBER(%s) cannot be converted to int, err=%v",
				buildStr, err)
		}

		prData := githubPr.New(*githubTokenPath, repoOwner, repoName, pr, *postingBotUserName)
		gcsData := &gcs.PresubmitBuild{GcsBuild: gcs.GcsBuild{
			Client:       gcs.NewClient(prData.Ctx),
			Bucket:       *gcsBucketName,
			Job:          jobName,
			Build:        build,
			CovThreshold: *covThreshold,
		},
			PostSubmitJob: *postSubmitJobName,
		}
		presubmit := &gcs.PreSubmit{
			GithubPr:       *prData,
			PresubmitBuild: *gcsData,
		}

		presubmit.Artifacts = *presubmit.MakeGcsArtifacts(*localArtifacts)
		isCoverageLow, err := RunPresubmit(presubmit, localArtifacts)
		if isCoverageLow {
			logUtil.LogFatalf("Code coverage is below threshold (%d%%), "+
				"fail presubmit workflow intentionally", *covThreshold)
		}
		if err != nil {
			log.Fatal(err)
		}
	case "periodic":
		log.Printf("job type is %v, producing testsuite xml...\n", jobType)
		testgrid.ProfileToTestsuiteXML(localArtifacts, *covThreshold)
	}

	fmt.Println("end of code coverage main")
}
