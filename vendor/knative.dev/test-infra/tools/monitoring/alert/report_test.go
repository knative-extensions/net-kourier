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

package alert

import (
	"testing"
	"time"

	"knative.dev/test-infra/tools/monitoring/mysql"
)

func TestSprintLogs(t *testing.T) {
	testTime, _ := time.Parse(time.RFC3339, "2019-01-01T00:00:00Z")
	tests := []struct {
		name string
		uut  report
		want string
	}{
		{
			name: "mixed valid and invalid gcsURL",
			uut: report{
				logs: []mysql.ErrorLog{
					{
						Pattern:     "Knative test failed",
						Msg:         "Knative test failed",
						JobName:     "ci-knative-docs-continuous",
						PRNumber:    0,
						BuildLogURL: "https://prow.knative.dev/view/gcs/knative-prow/pr-logs/1153085516762058753",
						TimeStamp:   testTime,
					},
					{
						Pattern:     "Knative test failed",
						Msg:         "Knative test failed",
						JobName:     "ci-knative-docs-continuous",
						PRNumber:    0,
						BuildLogURL: "%zzzzz",
						TimeStamp:   testTime,
					},
				},
			},
			want: "1. [2019-01-01 00:00:00 +0000 UTC] Knative test failed (Job: ci-knative-docs-continuous, PR: 0, BuildLog: https://prow.knative.dev/view/gcs/knative-prow/pr-logs/1153085516762058753)\n" +
				"2. [2019-01-01 00:00:00 +0000 UTC] Knative test failed (Job: ci-knative-docs-continuous, PR: 0, BuildLog: %zzzzz)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uut.sprintLogs(); got != tt.want {
				t.Errorf("(%v).sprintLog = %v, want: %v", tt.uut, got, tt.want)
			}
		})
	}
}
