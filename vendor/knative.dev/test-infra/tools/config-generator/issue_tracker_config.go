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

// data definitions that are used for the config file generation of issue tracker prow jobs

package main

import (
	"fmt"
	"strings"
)

const (
	staleDefault = 90
	closeDefault = 30
	rotDefault   = 30
	jobCmd       = "/app/robots/commenter/app.binary"
	periodicCron = "0 */12 * * *" // Run every 12 hours
)

var (
	feedbackNote = "Send feedback to [Knative Productivity Slack channel](https://knative.slack.com/messages/CCSNR4FCH) " +
		"or file an issue in [knative/test-infra](https://github.com/knative/test-infra/issues/new)."
)

type repoIssue struct {
	name        string
	daysToStale int
	daysToRot   int
	daysToClose int
}

// generateIssueTrackerPeriodicJobs generates the periodic issue tracker jobs to automatically manage issue lifecycles.
// It's a mirror of K8S fejta bot https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes/test-infra/fejta-bot-periodics.yaml.
func generateIssueTrackerPeriodicJobs() {
	// generate for knative/test-infra
	repoIssue{
		name:        "knative/test-infra",
		daysToStale: staleDefault,
		daysToRot:   rotDefault,
		daysToClose: closeDefault,
	}.generateJobs()

	// generate for knative/docs
	repoIssue{
		name:        "knative/docs",
		daysToStale: staleDefault,
		daysToRot:   rotDefault,
		daysToClose: closeDefault,
	}.generateJobs()

	// generate for knative/serving
	repoIssue{
		name:        "knative/serving",
		daysToStale: staleDefault,
		daysToRot:   rotDefault,
		daysToClose: closeDefault,
	}.generateJobs()

}

// generateJobs generates all the issue tracker jobs per repoIssue
func (r repoIssue) generateJobs() {
	repoForJob := strings.Replace(r.name, "/", "-", -1)
	jobName := fmt.Sprintf("ci-%s-issue-tracker-stale", repoForJob)
	// Do not look at issues that has frozen, stale or rotten label
	filter := `
        -label:lifecycle/frozen
        -label:lifecycle/stale
        -label:lifecycle/rotten`
	updatedTime := fmt.Sprintf("%dh", r.daysToStale*24)
	comment := fmt.Sprintf("--comment=Issues go stale after %d days of inactivity.\\n"+
		"Mark the issue as fresh by adding the comment `/remove-lifecycle stale`.\\n"+
		"Stale issues rot after an additional %d days of inactivity and eventually close.\\n"+
		"If this issue is safe to close now please do so by adding the comment `/close`.\\n\\n"+
		"%s\\n\\n"+
		"/lifecycle stale", r.daysToStale, r.daysToRot, feedbackNote)
	r.generateJob(jobName, filter, updatedTime, comment)

	jobName = fmt.Sprintf("ci-%s-issue-tracker-rotten", repoForJob)
	// Do not look at issues that has frozen or rotten label. Only look at stale labelled issues
	filter = `
        -label:lifecycle/frozen
        label:lifecycle/stale
        -label:lifecycle/rotten`
	updatedTime = fmt.Sprintf("%dh", r.daysToRot*24)
	comment = fmt.Sprintf("--comment=Stale issues rot after %d days of inactivity.\\n"+
		"Mark the issue as fresh by adding the comment `/remove-lifecycle rotten`.\\n"+
		"Rotten issues close after an additional %d days of inactivity.\\n"+
		"If this issue is safe to close now please do so by adding the comment `/close`.\\n\\n"+
		"%s\\n\\n"+
		"/lifecycle rotten", r.daysToRot, r.daysToClose, feedbackNote)
	r.generateJob(jobName, filter, updatedTime, comment)

	jobName = fmt.Sprintf("ci-%s-issue-tracker-close", repoForJob)
	// Do not look at issues that has frozen label. Only look at rotten labels
	filter = `
        -label:lifecycle/frozen
        -label:lifecycle/stale
        label:lifecycle/rotten`
	updatedTime = fmt.Sprintf("%dh", r.daysToClose*24)
	comment = fmt.Sprintf("--comment=Rotten issues close after %d days of inactivity.\\n"+
		"Reopen the issue with `/reopen`.\\n"+
		"Mark the issue as fresh by adding the comment `/remove-lifecycle rotten`.\\n\\n"+
		"%s\\n\\n"+
		"/close", r.daysToClose, feedbackNote)
	r.generateJob(jobName, filter, updatedTime, comment)
}

// generateJob generates a single issue tracker prow job
func (r repoIssue) generateJob(jobName, labelFilter, updatedTime, comment string) {
	var data periodicJobTemplateData
	data.Base = newbaseProwJobTemplateData("knative/test-infra")
	data.Base.ExtraRefs = append(data.Base.ExtraRefs, "  base_ref: "+data.Base.RepoBranch)
	data.Base.Image = githubCommenterDockerImage
	data.PeriodicJobName = jobName
	data.CronString = periodicCron
	data.Base.Command = jobCmd
	data.Base.Args = []string{
		fmt.Sprintf(`--query=repo:%s
        is:open
        %s`, r.name, labelFilter),
		"--updated=" + updatedTime,
		"--token=/etc/housekeeping-github-token/token",
		comment,
		"--template",
		"--ceiling=10",
		"--confirm",
	}
	addVolumeToJob(&data.Base, "/etc/housekeeping-github-token", "housekeeping-github-token", true, "")
	executeJobTemplate(jobName, readTemplate(periodicCustomJob), "presubmits", "", data.PeriodicJobName, false, data)
}
