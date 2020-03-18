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

// TODO(yt3liu): Remove this file after using definition in "k8s.io/test-infra" directly
// https://github.com/knative/test-infra/issues/912

package prowapi

// ProwJobType specifies how the job is triggered.
type ProwJobType string

// Various job types.
const (
	// PresubmitJob means it runs on unmerged PRs.
	PresubmitJob ProwJobType = "presubmit"
	// PostsubmitJob means it runs on each new commit.
	PostsubmitJob ProwJobType = "postsubmit"
	// Periodic job means it runs on a time-basis, unrelated to git changes.
	PeriodicJob ProwJobType = "periodic"
	// BatchJob tests multiple unmerged PRs at the same time.
	BatchJob ProwJobType = "batch"
)

// ProwJobState specifies whether the job is running
type ProwJobState string

// Various job states.
const (
	// TriggeredState means the job has been created but not yet scheduled.
	TriggeredState ProwJobState = "triggered"
	// PendingState means the job is scheduled but not yet running.
	PendingState ProwJobState = "pending"
	// SuccessState means the job completed without error (exit 0)
	SuccessState ProwJobState = "success"
	// FailureState means the job completed with errors (exit non-zero)
	FailureState ProwJobState = "failure"
	// AbortedState means prow killed the job early (new commit pushed, perhaps).
	AbortedState ProwJobState = "aborted"
	// ErrorState means the job could not schedule (bad config, perhaps).
	ErrorState ProwJobState = "error"
)

// Refs describes how the repo was constructed.
type Refs struct {
	// Org is something like kubernetes or k8s.io
	Org string `json:"org"`
	// Repo is something like test-infra
	Repo string `json:"repo"`
	// RepoLink links to the source for Repo.
	RepoLink string `json:"repo_link,omitempty"`

	BaseRef string `json:"base_ref,omitempty"`
	BaseSHA string `json:"base_sha,omitempty"`
	// BaseLink is a link to the commit identified by BaseSHA.
	BaseLink string `json:"base_link,omitempty"`

	Pulls []Pull `json:"pulls,omitempty"`

	// PathAlias is the location under <root-dir>/src
	// where this repository is cloned. If this is not
	// set, <root-dir>/src/github.com/org/repo will be
	// used as the default.
	PathAlias string `json:"path_alias,omitempty"`
	// CloneURI is the URI that is used to clone the
	// repository. If unset, will default to
	// `https://github.com/org/repo.git`.
	CloneURI string `json:"clone_uri,omitempty"`
	// SkipSubmodules determines if submodules should be
	// cloned when the job is run. Defaults to true.
	SkipSubmodules bool `json:"skip_submodules,omitempty"`
	// CloneDepth is the depth of the clone that will be used.
	// A depth of zero will do a full clone.
	CloneDepth int `json:"clone_depth,omitempty"`
}

// Pull describes a pull request at a particular point in time.
type Pull struct {
	Number int    `json:"number"`
	Author string `json:"author"`
	SHA    string `json:"sha"`
	Title  string `json:"title,omitempty"`

	// Ref is git ref can be checked out for a change
	// for example,
	// github: pull/123/head
	// gerrit: refs/changes/00/123/1
	Ref string `json:"ref,omitempty"`
	// Link links to the pull request itself.
	Link string `json:"link,omitempty"`
	// CommitLink links to the commit identified by the SHA.
	CommitLink string `json:"commit_link,omitempty"`
	// AuthorLink links to the author of the pull request.
	AuthorLink string `json:"author_link,omitempty"`
}

// ReportMessage is a message structure used to pass a prowjob status to Pub/Sub topic.s
type ReportMessage struct {
	Project string       `json:"project"`
	Topic   string       `json:"topic"`
	RunID   string       `json:"runid"`
	Status  ProwJobState `json:"status"`
	URL     string       `json:"url"`
	GCSPath string       `json:"gcs_path"`
	Refs    []Refs       `json:"refs,omitempty"`
	JobType ProwJobType  `json:"job_type"`
	JobName string       `json:"job_name"`
}
