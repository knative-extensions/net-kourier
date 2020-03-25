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

// data definitions that are used for the config file generation of performance
// tests cluster maintenance jobs.

package main

import (
	"fmt"
)

const (
	perfTestScriptPath = "./test/performance/performance-tests.sh"
	perfTestSecretName = "performance-test"
)

// generatePerfClusterUpdatePeriodicJobs generates periodic jobs to update clusters
// that run performance testing benchmarks
func generatePerfClusterUpdatePeriodicJobs() {
	for _, repo := range repositories {
		if repo.EnablePerformanceTests {
			perfClusterPeriodicJob(
				"recreate-clusters",
				recreatePerfClusterPeriodicJobCron,
				perfTestScriptPath,
				[]string{"--recreate-clusters"},
				repo,
				perfTestSecretName,
			)
			perfClusterPeriodicJob(
				"update-clusters",
				updatePerfClusterPeriodicJobCron,
				perfTestScriptPath,
				[]string{"--update-clusters"},
				repo,
				perfTestSecretName,
			)
		}
	}
}

// generatePerfClusterPostsubmitJob generates postsubmit job for the
// repo to reconcile clusters that run performance testing benchmarks.
func generatePerfClusterPostsubmitJob(repo repositoryData) {
	perfClusterReconcilePostsubmitJob(
		"reconcile-clusters",
		perfTestScriptPath,
		[]string{"--reconcile-benchmark-clusters"},
		repo,
		perfTestSecretName,
	)
}

func perfClusterPeriodicJob(jobNamePostFix, cronString, command string, args []string, repo repositoryData, sa string) {
	var data periodicJobTemplateData
	data.Base = perfClusterBaseProwJob(command, args, repo.Name, sa)
	data.Base.ExtraRefs = append(data.Base.ExtraRefs, "  base_ref: "+data.Base.RepoBranch)
	if repo.DotDev {
		data.Base.ExtraRefs = append(data.Base.ExtraRefs, "  path_alias: knative.dev/"+data.Base.RepoName)
	}
	data.PeriodicJobName = fmt.Sprintf("ci-%s-%s", data.Base.RepoNameForJob, jobNamePostFix)
	data.CronString = cronString
	data.PeriodicCommand = createCommand(data.Base)
	addMonitoringPubsubLabelsToJob(&data.Base, data.PeriodicJobName)
	executeJobTemplate("performance tests periodic", readTemplate(periodicTestJob),
		"periodics", repo.Name, data.PeriodicJobName, false, data)
}

func perfClusterReconcilePostsubmitJob(jobNamePostFix, command string, args []string, repo repositoryData, sa string) {
	var data postsubmitJobTemplateData
	data.Base = perfClusterBaseProwJob(command, args, repo.Name, sa)
	if repo.DotDev {
		data.Base.PathAlias = "path_alias: knative.dev/" + data.Base.RepoName
	}
	data.PostsubmitJobName = fmt.Sprintf("post-%s-%s", data.Base.RepoNameForJob, jobNamePostFix)
	data.PostsubmitCommand = createCommand(data.Base)
	addMonitoringPubsubLabelsToJob(&data.Base, data.PostsubmitJobName)
	executeJobTemplate("performance tests postsubmit", readTemplate(perfPostsubmitJob),
		"postsubmits", repo.Name, data.PostsubmitJobName, true, data)
}

func perfClusterBaseProwJob(command string, args []string, fullRepoName, sa string) baseProwJobTemplateData {
	base := newbaseProwJobTemplateData(fullRepoName)
	for _, repo := range repositories {
		if fullRepoName == repo.Name && repo.Go113 {
			base.Image = getGo113ImageName(base.Image)
			break
		}
	}

	base.Command = command
	base.Args = args
	addVolumeToJob(&base, "/etc/performance-test", sa, true, "")
	addEnvToJob(&base, "GOOGLE_APPLICATION_CREDENTIALS", "/etc/performance-test/service-account.json")
	addEnvToJob(&base, "GITHUB_TOKEN", "/etc/performance-test/github-token")
	addEnvToJob(&base, "SLACK_READ_TOKEN", "/etc/performance-test/slack-read-token")
	addEnvToJob(&base, "SLACK_WRITE_TOKEN", "/etc/performance-test/slack-write-token")
	return base
}
