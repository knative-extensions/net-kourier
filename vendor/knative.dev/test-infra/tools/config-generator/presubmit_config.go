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
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	// presubmitJob is the template for presubmit jobs.
	presubmitJob = "prow_presubmit_job.yaml"

	// presubmitGoCoverageJob is the template for go coverage presubmit jobs.
	presubmitGoCoverageJob = "prow_presubmit_gocoverate_job.yaml"
)

// presubmitJobTemplateData contains data about a presubmit Prow job.
type presubmitJobTemplateData struct {
	Base                 baseProwJobTemplateData
	PresubmitJobName     string
	PresubmitPullJobName string
	PresubmitPostJobName string
	PresubmitCommand     []string
}

// generatePresubmit generates all presubmit job configs for the given repo and configuration.
func generatePresubmit(title string, repoName string, presubmitConfig yaml.MapSlice) {
	var data presubmitJobTemplateData
	data.Base = newbaseProwJobTemplateData(repoName)
	data.Base.Command = presubmitScript
	data.Base.GoCoverageThreshold = 50
	jobTemplate := readTemplate(presubmitJob)
	repoData := repositoryData{Name: repoName, EnableGoCoverage: false, GoCoverageThreshold: data.Base.GoCoverageThreshold}
	generateJob := true
	for i, item := range presubmitConfig {
		switch item.Key {
		case "build-tests", "unit-tests", "integration-tests":
			if !getBool(item.Value) {
				return
			}
			jobName := getString(item.Key)
			data.PresubmitJobName = data.Base.RepoNameForJob + "-" + jobName
			// Use default arguments if none given.
			if len(data.Base.Args) == 0 {
				data.Base.Args = []string{"--" + jobName}
			}
			addVolumeToJob(&data.Base, "/etc/repoview-token", "repoview-token", true, "")
		case "go-coverage":
			if !getBool(item.Value) {
				return
			}
			jobTemplate = readTemplate(presubmitGoCoverageJob)
			data.PresubmitJobName = data.Base.RepoNameForJob + "-go-coverage"
			data.Base.Image = coverageDockerImage
			data.Base.ServiceAccount = ""
			repoData.EnableGoCoverage = true
			addVolumeToJob(&data.Base, "/etc/covbot-token", "covbot-token", true, "")
		case "custom-test":
			data.PresubmitJobName = data.Base.RepoNameForJob + "-" + getString(item.Value)
		case "go-coverage-threshold":
			data.Base.GoCoverageThreshold = getInt(item.Value)
			repoData.GoCoverageThreshold = data.Base.GoCoverageThreshold
		case "repo-settings":
			generateJob = false
		default:
			continue
		}
		// Knock-out the item, signalling it was already parsed.
		presubmitConfig[i] = yaml.MapItem{}
	}
	repositories = append(repositories, repoData)
	parseBasicJobConfigOverrides(&data.Base, presubmitConfig)
	if !generateJob {
		return
	}
	data.PresubmitCommand = createCommand(data.Base)
	data.PresubmitPullJobName = "pull-" + data.PresubmitJobName
	data.PresubmitPostJobName = "post-" + data.PresubmitJobName
	if data.Base.ServiceAccount != "" {
		addEnvToJob(&data.Base, "GOOGLE_APPLICATION_CREDENTIALS", data.Base.ServiceAccount)
		addEnvToJob(&data.Base, "E2E_CLUSTER_REGION", "us-central1")
	}
	if data.Base.NeedsMonitor {
		addMonitoringPubsubLabelsToJob(&data.Base, data.PresubmitPullJobName)
	}
	addExtraEnvVarsToJob(extraEnvVars, &data.Base)
	configureServiceAccountForJob(&data.Base)
	jobName := data.PresubmitPullJobName
	executeJobTemplateWrapper(repoName, &data, func(data interface{}) {
		executeJobTemplate("presubmit", jobTemplate, title, repoName, jobName, true, data)
	})
	// Generate config for pull-knative-serving-go-coverage-dev right after pull-knative-serving-go-coverage,
	// this job is mainly for debugging purpose.
	if data.PresubmitPullJobName == "pull-knative-serving-go-coverage" {
		data.PresubmitPullJobName += "-dev"
		data.Base.AlwaysRun = false
		data.Base.Image = strings.Replace(data.Base.Image, "coverage:latest", "coverage-dev:latest", -1)
		data.Base.Image = strings.Replace(data.Base.Image, "coverage-go112:latest", "coverage-dev:latest", -1)
		template := strings.Replace(readTemplate(presubmitGoCoverageJob), "(all|", "(", 1)
		executeJobTemplate("presubmit", template, title, repoName, data.PresubmitPullJobName, true, data)
	}
}
