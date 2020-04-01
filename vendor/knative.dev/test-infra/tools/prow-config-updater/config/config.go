/*
Copyright 2020 The Knative Authors

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

package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"knative.dev/pkg/test/cmd"
	"knative.dev/pkg/test/helpers"
)

type ProwEnv string

const (
	ProdProwEnv    ProwEnv = "prow"
	StagingProwEnv ProwEnv = "prow-staging"
)

const (
	OrgName  = "knative"
	RepoName = "test-infra"
	PRHead   = "autoupdate"
	PRBase   = "master"
	// the label that needs to be applied on the PR to get it automatically merged
	AutoMergeLabel = "auto-merge"

	PullBotName          = "pull[bot]"
	PullEndpointTemplate = "https://pull.git.ci/process/%s/%s"

	configPath         = "config"
	configTemplatePath = "tools/config-generator/templates"

	// Prow config subfolder names
	core    = "core"
	jobs    = "jobs"
	cluster = "cluster"
)

var (
	// Prow config root paths.
	ProdProwConfigRoot    = filepath.Join(configPath, string(ProdProwEnv))
	StagingProwConfigRoot = filepath.Join(configPath, string(StagingProwEnv))
	// Prow config templates paths.
	ProdProwConfigTemplatesPath    = filepath.Join(configTemplatePath, string(ProdProwEnv))
	StagingProwConfigTemplatesPath = filepath.Join(configTemplatePath, string(StagingProwEnv))

	// Commands that generate and update Prow configs.
	// These are commands for both staging and production Prow.
	generateConfigFilesCommand = "./hack/generate-configs.sh"
	updateProwCommandTemplate  = "make -C %s update-prow-cluster"
	updateProdProwCommand      = fmt.Sprintf(updateProwCommandTemplate, ProdProwConfigRoot)
	updateStagingProwCommand   = fmt.Sprintf(updateProwCommandTemplate, StagingProwConfigRoot)
	// This command is only used for production prow in this tool.
	updateTestgridCommand = fmt.Sprintf("make -C %s update-testgrid-config", ProdProwConfigRoot)

	// Config paths that need to be handled by prow-config-updater if files under them are changed.
	ProdProwConfigPaths = []string{
		filepath.Join(ProdProwConfigRoot, core),
		filepath.Join(ProdProwConfigRoot, jobs),
		filepath.Join(ProdProwConfigRoot, cluster),
	}
	StagingProwConfigPaths = []string{
		filepath.Join(StagingProwConfigRoot, core),
		filepath.Join(StagingProwConfigRoot, jobs),
		filepath.Join(StagingProwConfigRoot, cluster),
	}
	ProdTestgridConfigPath = filepath.Join(ProdProwConfigRoot, "testgrid")

	// Config paths that need to be gated and tested by prow-config-updater.
	ProdProwKeyConfigPaths = []string{
		filepath.Join(ProdProwConfigRoot, cluster),
		ProdProwConfigTemplatesPath,
	}
	StagingProwKeyConfigPaths = []string{
		filepath.Join(StagingProwConfigRoot, cluster),
		StagingProwConfigTemplatesPath,
	}
)

// UpdateProw will update Prow with the existing Prow config files.
func UpdateProw(env ProwEnv, dryrun bool) error {
	updateCommand := ""
	switch env {
	case ProdProwEnv:
		updateCommand = updateProdProwCommand
	case StagingProwEnv:
		updateCommand = updateStagingProwCommand
	default:
		return fmt.Errorf("unsupported Prow environement: %q, cannot make the update", env)
	}

	return helpers.Run(
		fmt.Sprintf("Updating Prow configs with command %q", updateCommand),
		func() error {
			// Use the default GOOGLE_APPLICATION_CREDENTIALS to authenticate with the gcloud services,
			// it will fallback to use the local credentials if the env var does not exist.
			kf := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
			if kf != "" {
				authCommand := "gcloud auth activate-service-account --key-file=" + kf
				if _, err := cmd.RunCommand(authCommand); err != nil {
					return fmt.Errorf("error activating service account with %q", kf)
				}
			}
			out, err := cmd.RunCommand(updateCommand, cmd.WithEnvs(os.Environ()))
			log.Println(out)
			return err
		},
		dryrun,
	)
}

// UpdateTestgrid will update testgrid with the existing testgrid config file.
func UpdateTestgrid(env ProwEnv, dryrun bool) error {
	if env != ProdProwEnv {
		log.Printf("no testgrid config needs to be updated for %q Prow environment", env)
		return nil
	}

	return helpers.Run(
		fmt.Sprintf("Updating Testgrid config with command %q", updateTestgridCommand),
		func() error {
			out, err := cmd.RunCommand(updateTestgridCommand, cmd.WithEnvs(os.Environ()))
			log.Println(out)
			return err
		},
		dryrun,
	)
}

// GenerateConfigFiles will run the config generator command to generate new Prow config files.
func GenerateConfigFiles() error {
	_, err := cmd.RunCommand(generateConfigFilesCommand)
	return err
}

// CollectRelevantFiles can filter out all files that are under the given paths.
func CollectRelevantFiles(files []string, paths []string) []string {
	rfs := make([]string, 0)
	for _, f := range files {
		for _, p := range paths {
			if !strings.HasSuffix(p, string(filepath.Separator)) {
				p = p + string(filepath.Separator)
			}
			if strings.HasPrefix(f, p) {
				rfs = append(rfs, f)
			}
		}
	}
	return rfs
}
