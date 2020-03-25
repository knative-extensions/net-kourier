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
package artifacts

import (
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"knative.dev/test-infra/tools/coverage/logUtil"
)

type LocalArtifacts struct {
	Artifacts
}

func NewLocalArtifacts(directory string, ProfileName string,
	KeyProfileName string, CovStdoutName string) *LocalArtifacts {
	return &LocalArtifacts{*New(
		directory,
		ProfileName,
		KeyProfileName,
		CovStdoutName)}
}

// ProfileReader create and returns a ProfileReader by opening the file stored in profile path
func (arts *LocalArtifacts) ProfileReader() *ProfileReader {
	f, err := os.Open(arts.ProfilePath())
	if err != nil {
		wd, _ := os.Getwd()
		logUtil.LogFatalf("LocalArtifacts.ProfileReader(): os.Open(profilePath) error: %v, cwd=%s", err, wd)
	}
	return NewProfileReader(f)
}

func (arts *LocalArtifacts) ProfileName() string {
	return arts.profileName
}

// KeyProfileCreator creates a key profile file that will be used to hold a
// filtered version of coverage profile that only stores the entries that
// will be displayed by line coverage tool
func (arts *LocalArtifacts) KeyProfileCreator() *os.File {
	keyProfilePath := arts.KeyProfilePath()
	keyProfileFile, err := os.Create(keyProfilePath)
	log.Printf("os.Create(keyProfilePath)=%s", keyProfilePath)
	if err != nil {
		logUtil.LogFatalf("file(%s) creation error: %v", keyProfilePath, err)
	}

	return keyProfileFile
}

// ProduceProfileFile produce coverage profile (&its stdout) by running go test on target package
// for periodic job, produce junit xml for testgrid in addition
func (arts *LocalArtifacts) ProduceProfileFile(covTargetsStr string) {
	// creates artifacts directory
	log.Printf("mkdir -p %s\n", arts.directory)
	cmd := exec.Command("mkdir", "-p", arts.directory)
	log.Printf("artifacts dir=%s\n", arts.directory)
	cmd.Run()

	// convert targets from a single string to a lists of strings
	var covTargets []string
	for _, target := range strings.Split(covTargetsStr, " ") {
		covTargets = append(covTargets, "./"+path.Join(target, "..."))
	}
	log.Printf("covTargets = %v\n", covTargets)

	runProfiling(covTargets, arts)
}
