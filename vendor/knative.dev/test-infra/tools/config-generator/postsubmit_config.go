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
	"path"
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	// goCoveragePostsubmitJob is the template for the go postsubmit coverage job.
	goCoveragePostsubmitJob = "prow_postsubmit_gocoverage_job.yaml"

	// perfPostsubmitJob is the template for the performance operations
	// postsubmit job.
	perfPostsubmitJob = "prow_postsubmit_perf_job.yaml"
)

// postsubmitJobTemplateData contains data about a postsubmit Prow job.
type postsubmitJobTemplateData struct {
	Base              baseProwJobTemplateData
	PostsubmitJobName string
	PostsubmitCommand []string
}

// generateGoCoveragePostsubmit generates the go coverage postsubmit job config for the given repo.
func generateGoCoveragePostsubmit(title, repoName string, _ yaml.MapSlice) {
	var data postsubmitJobTemplateData
	data.Base = newbaseProwJobTemplateData(repoName)
	data.Base.Image = coverageDockerImage
	data.PostsubmitJobName = fmt.Sprintf("post-%s-go-coverage", data.Base.RepoNameForJob)
	for _, repo := range repositories {
		if repo.Name == repoName && repo.DotDev {
			data.Base.PathAlias = "path_alias: knative.dev/" + path.Base(repoName)
		}
		if repo.Name == repoName && repo.Go113 {
			data.Base.Image = getGo113ImageName(data.Base.Image)
		}
	}
	addExtraEnvVarsToJob(extraEnvVars, &data.Base)
	configureServiceAccountForJob(&data.Base)
	jobName := data.PostsubmitJobName
	executeJobTemplateWrapper(repoName, &data, func(data interface{}) {
		executeJobTemplate("postsubmit go coverage", readTemplate(goCoveragePostsubmitJob), title, repoName, jobName, true, data)
	})
	// Generate config for post-knative-serving-go-coverage-dev right after post-knative-serving-go-coverage,
	// this job is mainly for debugging purpose.
	if data.PostsubmitJobName == "post-knative-serving-go-coverage" {
		data.PostsubmitJobName += "-dev"
		data.Base.Image = strings.Replace(data.Base.Image, "coverage-go112:latest", "coverage-dev:latest", -1)
		data.Base.Image = strings.Replace(data.Base.Image, "coverage:latest", "coverage-dev:latest", -1)
		executeJobTemplate("postsubmit go coverage", readTemplate(goCoveragePostsubmitJob), title, repoName, data.PostsubmitJobName, false, data)
	}
}
