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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"knative.dev/test-infra/pkg/common"
)

func TestProwConfigPathsExist(t *testing.T) {
	pathsToCheck := [][]string{ProdProwConfigPaths, StagingProwKeyConfigPaths, {ProdTestgridConfigPath}}
	checkPaths(pathsToCheck, t)
}

func TestProwKeyConfigPathsExist(t *testing.T) {
	pathsToCheck := [][]string{ProdProwKeyConfigPaths, StagingProwKeyConfigPaths}
	checkPaths(pathsToCheck, t)
}

func checkPaths(pathsArr [][]string, t *testing.T) {
	t.Helper()
	root, err := common.GetRootDir()
	if err != nil {
		t.Fatalf("Failed to get the root dir: %v", err)
	}
	for _, paths := range pathsArr {
		for _, p := range paths {
			info, err := os.Stat(filepath.Join(root, p))
			if os.IsNotExist(err) || !info.IsDir() {
				t.Fatalf("Expected %q to be a dir, but it's not: %v", p, err)
			}
		}
	}
}

func TestCollectRelevantFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		paths    []string
		expected []string
	}{{
		name:     "no relevant files",
		files:    []string{"test/file1.yaml", "test1/file2.yaml"},
		paths:    []string{"test3"},
		expected: []string{},
	}, {
		name:     "relevant files from one dir",
		files:    []string{"test/file1.yaml", "test2/file2.yaml"},
		paths:    []string{"test"},
		expected: []string{"test/file1.yaml"},
	}, {
		name:     "relevant files from multiple dirs",
		files:    []string{"test/file1.yaml", "test2/file2.yaml", "test3/file3.yaml"},
		paths:    []string{"test", "test2"},
		expected: []string{"test/file1.yaml", "test2/file2.yaml"},
	}, {
		name:     "relevant files from root dir",
		files:    []string{"test/test1/file1.yaml", "test/test2/file2.yaml", "test111/file3.yaml"},
		paths:    []string{"test"},
		expected: []string{"test/test1/file1.yaml", "test/test2/file2.yaml"},
	}}

	for _, test := range tests {
		res := CollectRelevantFiles(test.files, test.paths)
		cmpRes := cmp.Diff(res, test.expected)
		if cmpRes != "" {
			t.Fatalf("expect and actual are different:\n%s", cmpRes)
		}
	}
}
