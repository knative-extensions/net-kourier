/*
Copyright 2019 The Knative Authors

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
	"fmt"
	"regexp"
	"time"

	"github.com/google/go-github/github"
	"knative.dev/pkg/test/ghutil"
)

const (
	// Git info for k8s Prow auto bumper PRs
	srcOrg  = "kubernetes"
	srcRepo = "test-infra"
	// srcPRHead is the head branch of k8s auto version bump PRs
	// TODO(chaodaiG): using head branch querying is less ideal than using
	// label `area/prow/bump`, which is not supported by Github API yet. Move
	// to filter using this label once it's supported
	srcPRHead = "autobump"
	// srcPRBase is the base branch of k8s auto version bump PRs
	srcPRBase = "master"
	// srcPRUserID is the user from which PR was created
	srcPRUserID = "k8s-ci-robot"

	// Git info for target repo that Prow version bump PR targets
	org  = "knative"
	repo = "test-infra"
	// PRHead is branch name where the changes occur
	PRHead = "autobump"
	// PRBase is the branch name where PR targets
	PRBase = "master"

	// Index for regex matching groups
	imageRootPart = 1 // first group is image root folder part
	imageSubPart  = 2 // second group is the optional subfolder in the image path
	imageTagPart  = 3 // third group is tag part
	// Max delta away from target date
	maxDelta = 2 * 24 // 2 days
	// K8s updates Prow versions everyday, which should be ~24 hours,
	// if a version is updated within 12 hours, it's considered not safe
	safeDuration = 12 // 12 hours
	maxRetry     = 3

	oncallAddress = "https://storage.googleapis.com/knative-infra-oncall/oncall.json"
)

var (
	// configPaths are the list of paths where the configuration files are saved
	configPaths = []string{"config", "tools/config-generator"}
	// Whitelist of files to be scanned by this tool
	fileFilters = []*regexp.Regexp{regexp.MustCompile(`\.yaml$`)}
	// Matching            gcr.io /k8s-(prow|testimage)/(kubekin-e2e|boskos|.*) (/janitor|/reaper|/.*)?        :vYYYYMMDD-HASH-VARIANT
	imagePattern     = `\b(gcr\.io/k8s[a-z0-9-]{5,29}/[a-zA-Z0-9][a-zA-Z0-9_.-]+(/[a-zA-Z0-9][a-zA-Z0-9_.-]+)?):(v[a-zA-Z0-9_.-]+)\b`
	imageRegexp      = regexp.MustCompile(imagePattern)
	imageLinePattern = fmt.Sprintf(`\s+[a-z]+:\s+"?'?%s"?'?`, imagePattern)
	// Matching   "-    image: gcr.io /k8s-(prow|testimage)/(tide|kubekin-e2e|.*)    :vYYYYMMDD-HASH-VARIANT"
	imageMinusRegexp = regexp.MustCompile(fmt.Sprintf(`\-%s`, imageLinePattern))
	// Matching   "+    image: gcr.io /k8s-(prow|testimage)/(tide|kubekin-e2e|.*)    :vYYYYMMDD-HASH-VARIANT"
	imagePlusRegexp = regexp.MustCompile(fmt.Sprintf(`\+%s`, imageLinePattern))
	// Preferred time for candidate PR creation date
	targetTime = time.Now().Add(-time.Hour * 7 * 24) // 7 days
)

// GHClientWrapper handles methods for github issues
type GHClientWrapper struct {
	ghutil.GithubOperations
}

// Versions holds the version change for an image
// oldVersion and newVersion are both in the format of "vYYYYMMDD-HASH-VARIANT"
type versions struct {
	oldVersion string
	newVersion string
	variant    string
}

// PRVersions contains PR and version changes in it
type PRVersions struct {
	images map[string][]versions // map of image name: versions struct
	// The way k8s updates versions doesn't guarantee the same version tag across all images,
	// dominantVersions is the version that appears most times
	dominantVersions *versions
	PR               *github.PullRequest
}

// Helper method for adding a newly discovered tag into pv
func (pv *PRVersions) getIndex(image, tag string) int {
	if _, ok := pv.images[image]; !ok {
		pv.images[image] = make([]versions, 0, 0)
	}
	_, variant := deconstructTag(tag)
	iv := -1
	for i, vs := range pv.images[image] {
		if vs.variant == variant {
			iv = i
			break
		}
	}
	if -1 == iv {
		pv.images[image] = append(pv.images[image], versions{variant: variant})
		iv = len(pv.images[image]) - 1
	}
	return iv
}
