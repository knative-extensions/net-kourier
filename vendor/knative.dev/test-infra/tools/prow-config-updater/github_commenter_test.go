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

package main

import (
	"testing"
)

func TestFileListCommentString(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{{
		name:     "empty list",
		files:    make([]string, 0),
		expected: "",
	}, {
		name:     "list with one string",
		files:    []string{"file1.yaml"},
		expected: "- `file1.yaml`",
	}, {
		name:     "list with multiple strings",
		files:    []string{"file1.yaml", "file2.yaml", "file3.yaml"},
		expected: "- `file1.yaml`\n- `file2.yaml`\n- `file3.yaml`",
	},
	}

	for _, test := range tests {
		res := fileListCommentString(test.files)
		if res != test.expected {
			t.Fatalf("expect: %q, actual: %q", test.expected, res)
		}
	}
}
