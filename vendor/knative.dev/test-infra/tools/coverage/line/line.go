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
// Package line provides line-by-line coverage
package line

import (
	"fmt"
	"log"
	"os/exec"

	"knative.dev/test-infra/tools/coverage/artifacts"
	"knative.dev/test-infra/tools/coverage/calc"
	"knative.dev/test-infra/tools/coverage/gcs"
)

func CreateLineCovFile(arts *artifacts.LocalArtifacts) error {
	pathKeyProfile := arts.KeyProfilePath()
	pathLineCov := arts.LineCovFilePath()
	cmdTxt := fmt.Sprintf("go tool cover -html=%s -o %s", pathKeyProfile, pathLineCov)
	log.Printf("Running command '%s'\n", cmdTxt)
	cmd := exec.Command("go", "tool", "cover", "-html="+pathKeyProfile, "-o", pathLineCov)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Printf("Finished running '%s'\n", cmdTxt)
	log.Printf("cmd.Args=%v", cmd.Args)
	if err != nil {
		log.Printf("Error executing cmd: %v; combinedOutput=%s", err, stdoutStderr)
	}
	return err
}

func GenerateLineCovLinks(
	presubmitBuild *gcs.PreSubmit, g *calc.CoverageList) {
	calc.SortCoverages(*g.Group())
	for i := 0; i < len(*g.Group()); i++ {
		g.Item(i).SetLineCovLink(presubmitBuild.UrlGcsLineCovLinkWithMarker(i))
		fmt.Printf("g.Item(i=%d).LineCovLink(): %s\n", i, g.Item(i).LineCovLink())
	}
}
