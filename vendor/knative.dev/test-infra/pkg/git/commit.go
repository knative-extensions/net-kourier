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

package git

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"knative.dev/pkg/test/helpers"
)

func call(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// MakeCommit adds the changed files and create a new Git commit.
func MakeCommit(gi Info, message string, dryrun bool) error {
	if gi.Head == "" {
		log.Fatal("pushing to empty branch ref is not allowed")
	}
	if err := helpers.Run(
		"Running 'git add -A'",
		func() error { return call("git", "add", "-A") },
		dryrun,
	); err != nil {
		return fmt.Errorf("failed to git add: %v", err)
	}
	commitArgs := []string{"commit", "-m", message}
	if gi.UserName != "" && gi.Email != "" {
		commitArgs = append(commitArgs, "--author", fmt.Sprintf("%s <%s>", gi.UserName, gi.Email))
	}
	if err := helpers.Run(
		fmt.Sprintf("Running 'git %s'", strings.Join(commitArgs, " ")),
		func() error { return call("git", commitArgs...) },
		dryrun,
	); err != nil {
		return fmt.Errorf("failed to git commit: %v", err)
	}
	pushArgs := []string{"push", "-f", fmt.Sprintf("git@github.com:%s/%s.git", gi.UserID, gi.Repo),
		fmt.Sprintf("HEAD:%s", gi.Head)}
	if err := helpers.Run(
		fmt.Sprintf("Running 'git %s'", strings.Join(pushArgs, " ")),
		func() error { return call("git", pushArgs...) },
		dryrun,
	); err != nil {
		return fmt.Errorf("failed to git push: %v", err)
	}
	return nil
}
