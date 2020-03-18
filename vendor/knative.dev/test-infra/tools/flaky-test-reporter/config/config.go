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

// config.go contains configurations for flaky tests reporting

package config

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

// configFile saves all information we need, this path is caller based
const configFile = "config/config.yaml"

var JobConfigs []JobConfig

// Config contains all job configs for flaky tests reporting
type Config struct {
	JobConfigs []JobConfig `yaml:"jobConfigs"`
}

// JobConfig is initial configuration for a given repo, defines which job to scan
type JobConfig struct {
	Name          string         `yaml:"name"` // name of job to analyze
	Repo          string         `yaml:"repo"` // repository to test job on
	Type          string         `yaml:"type"`
	IssueRepo     string         `yaml:"issueRepo,omitempty"`
	SlackChannels []SlackChannel `yaml:"slackChannels,omitempty"`
}

// SlackChannel contains Slack channels info
type SlackChannel struct {
	Name     string `yaml:"name"`
	Identity string `yaml:"identity"`
}

func init() {
	contents, err := ioutil.ReadFile(configFile)
	if err != nil {
		// If running in container the relative path would not work,
		// get current file dir and try to resolve it with Abs path
		dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
		contents, err = ioutil.ReadFile(filepath.Join(dir, configFile))
	}
	if err != nil {
		log.Printf("Failed to load the config file: %v", err)
		return
	}

	config := &Config{}
	if err = yaml.Unmarshal(contents, config); err != nil {
		log.Printf("Failed to unmarshal %v", contents)
	} else {
		JobConfigs = config.JobConfigs
	}
}
