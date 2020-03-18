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

package log_parser

import (
	"reflect"
	"regexp"
	"testing"

	"knative.dev/test-infra/tools/monitoring/mysql"
)

const (
	sampleLog = `I0515 00:00:32.675] ***************************************
I0515 00:00:32.675] ***         E2E TEST FAILED         ***
I0515 00:00:32.675] ***     End of information dump     ***
I0515 00:00:32.675] ***************************************
I0515 00:00:32.676] 2019/05/15 00:00:32 process.go:155: Step '/go/src/github.com/knative/docs/test/e2e-tests.sh --run-tests --emit-metrics' finished in 4m12.254697081s
I0515 00:00:32.676] 2019/05/15 00:00:32 main.go:312: Something went wrong: encountered 1 errors: [error during /go/src/github.com/knative/docs/test/e2e-tests.sh --run-tests --emit-metrics: exit status 1]
I0515 00:00:32.676] Test subprocess exited with code 0
I0515 00:00:32.676] Artifacts were written to /workspace/_artifacts
I0515 00:00:32.676] Test result code is 1
I0515 00:00:32.676] ==================================
I0515 00:00:32.676] ==== INTEGRATION TESTS FAILED ====
I0515 00:00:32.677] ==================================
W0515 00:00:32.677] Run: ('/workspace/./test-infra/jenkins/../scenarios/../hack/coalesce.py',)
E0515 00:00:32.677] Command failed`
)

func TestFindMatches(t *testing.T) {
	type args struct {
		regexps []regexp.Regexp
		text    []byte
	}
	tests := []struct {
		name string
		args args
		want []mysql.ErrorLog
	}{
		{
			name: "single match",
			args: args{
				regexps: []regexp.Regexp{
					*regexp.MustCompile("Something went wrong: starting e2e cluster: error creating cluster"),
					*regexp.MustCompile("sample*error2"),
					*regexp.MustCompile("Something went wrong:.*\n"),
				},
				text: []byte(sampleLog),
			},
			want: []mysql.ErrorLog{
				{
					Pattern: "Something went wrong:.*\n",
					Msg:     "Something went wrong: encountered 1 errors: [error during /go/src/github.com/knative/docs/test/e2e-tests.sh --run-tests --emit-metrics: exit status 1]\n",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collectMatches(tt.args.regexps, tt.args.text); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("find all matching errors: got = %v, want %v", got, tt.want)
			}
		})
	}
}
