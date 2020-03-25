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
	"io"
	"log"
	"os"
	"os/exec"

	covIo "knative.dev/test-infra/tools/coverage/io"
)

type ProfileReader struct {
	io.ReadCloser
}

func NewProfileReader(reader io.ReadCloser) *ProfileReader {
	return &ProfileReader{reader}
}

// runProfiling writes coverage profile (&its stdout) by running go test on
// target package
func runProfiling(covTargets []string, localArts *LocalArtifacts) {
	log.Println("\nStarts calc.runProfiling(...)")

	cmdArgs := []string{"test"}

	cmdArgs = append(cmdArgs, covTargets...)
	cmdArgs = append(cmdArgs, []string{"-covermode=count",
		"-coverprofile", localArts.ProfilePath()}...)

	log.Printf("go cmdArgs=%v\n", cmdArgs)
	cmd := exec.Command("go", cmdArgs...)

	output, errCmdOutput := cmd.CombinedOutput()

	if errCmdOutput != nil {
		log.Printf("Error running 'go test -coverprofile ': error='%v'; combined output='%s'\n",
			errCmdOutput, output)
	}

	log.Printf("coverage profile created @ '%s'", localArts.ProfilePath())
	covIo.CreateMarker(localArts.Directory(), CovProfileCompletionMarker)

	stdoutPath := localArts.CovStdoutPath()
	stdoutFile, err := os.Create(stdoutPath)
	if err == nil {
		stdoutFile.Write(output)
	} else {
		log.Printf("Error creating stdout file: %v", err)
	}
	defer stdoutFile.Close()
	log.Printf("stdout of test coverage stored in %s\n", stdoutPath)
	log.Printf("Ends calc.runProfiling(...)\n\n")
	return
}
