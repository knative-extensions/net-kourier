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
package githubUtil

import (
	"os"
	"testing"
)

func TestFilePathProfileToGithub(t *testing.T) {
	type args struct {
		file     string
		gopath   string
		repoRoot string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"repo on github.com",
			args{"github.com/myRepoOwner/myRepoName/pkg/ab/cde",
				"/d1/d2/d3/gopath",
				"/d1/d2/d3/gopath/src/github.com/myRepoOwner/myRepoName"},
			"pkg/ab/cde"},
		{"repo on knative.dev",
			args{"knative.dev/test-infra/pkg/ab/cde",
				"/d1/d2/gopath",
				"/d1/d2/gopath/src/knative.dev/test-infra"},
			"pkg/ab/cde"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gopath := os.Getenv("GOPATH")
			os.Setenv("GOPATH", tt.args.gopath)
			getRepoRootSaved := getRepoRoot
			getRepoRoot = func() (string, error) {
				return tt.args.repoRoot, nil
			}
			defer func() {
				os.Setenv("GOPATH", gopath)
				getRepoRoot = getRepoRootSaved
			}()
			if got := FilePathProfileToGithub(tt.args.file); got != tt.want {
				t.Errorf("FilePathProfileToGithub(%v) = %v, want %v", tt.args.file, got, tt.want)
			}
		})
	}
}
