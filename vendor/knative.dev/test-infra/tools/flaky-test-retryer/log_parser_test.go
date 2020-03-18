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

package main

import (
	"reflect"
	"testing"

	"knative.dev/test-infra/tools/flaky-test-reporter/jsonreport/fakejsonreport"
	"knative.dev/test-infra/tools/monitoring/prowapi"
)

var (
	fakeFlakyTests  = []string{"test0", "test1", "test2"}
	fakeInvalidRepo = &prowapi.ReportMessage{
		JobName: "fakejob",
		JobType: prowapi.PresubmitJob,
		Status:  prowapi.FailureState,
		Refs: []prowapi.Refs{{
			Org:  "fakeorg",
			Repo: "invalid",
			Pulls: []prowapi.Pull{{
				Number: 111,
			}},
		}},
	}
	fakeValidMessage = &prowapi.ReportMessage{
		JobName: "fakejob",
		JobType: prowapi.PresubmitJob,
		Status:  prowapi.FailureState,
		Refs: []prowapi.Refs{{
			Org:  "fakeorg",
			Repo: fakeRepo,
			Pulls: []prowapi.Pull{{
				Number: 111,
			}},
		}},
	}
)

func setup() {
	client, _ = fakejsonreport.Initialize("")
	client.CreateReport(fakeRepo, fakeFlakyTests, true)
}

func testIsSupported(t *testing.T) {
	cases := []struct {
		job  *JobData
		want bool
	}{
		{&JobData{&prowapi.ReportMessage{ // wrong state
			JobName: "fakejob",
			JobType: prowapi.PresubmitJob,
			Status:  prowapi.SuccessState,
			Refs: []prowapi.Refs{{
				Org:  "fakeorg",
				Repo: fakeRepo,
				Pulls: []prowapi.Pull{{
					Number: 111,
				}},
			}},
		}, nil, nil}, false},
		{&JobData{&prowapi.ReportMessage{ // wrong job type
			JobName: "fakejob",
			JobType: prowapi.PeriodicJob,
			Status:  prowapi.FailureState,
			Refs: []prowapi.Refs{{
				Org:  "fakeorg",
				Repo: fakeRepo,
				Pulls: []prowapi.Pull{{
					Number: 111,
				}},
			}},
		}, nil, nil}, false},
		{&JobData{&prowapi.ReportMessage{ // no refs
			JobName: "fakejob",
			JobType: prowapi.PresubmitJob,
			Status:  prowapi.FailureState,
			Refs:    nil,
		}, nil, nil}, false},
		{&JobData{&prowapi.ReportMessage{ // no pulls
			JobName: "fakejob",
			JobType: prowapi.PresubmitJob,
			Status:  prowapi.FailureState,
			Refs: []prowapi.Refs{{
				Org:   "fakeorg",
				Repo:  fakeRepo,
				Pulls: nil,
			}},
		}, nil, nil}, false},
		{&JobData{fakeInvalidRepo, nil, nil}, false}, // invalid repo
		{&JobData{fakeValidMessage, nil, nil}, true}, // valid message
	}
	setup()
	for _, test := range cases {
		got := test.job.IsSupported()
		if got != test.want {
			t.Fatalf("Is Supported: got %v, want %v", got, test.want)
		}
	}
}

func testGetFlakyTests(t *testing.T) {
	data := []struct {
		job       *JobData
		wantArray []string
		wantErr   error
	}{
		{&JobData{fakeValidMessage, nil, nil}, fakeFlakyTests, nil},
		{&JobData{fakeInvalidRepo, nil, nil}, []string{}, nil},
	}
	setup()
	for _, test := range data {
		gotArray, gotErr := test.job.getFlakyTests()
		if !reflect.DeepEqual(gotArray, test.wantArray) {
			t.Fatalf("Get Flaky Tests: got array %v, want array %v", gotArray, test.wantArray)
		}
		if gotErr != test.wantErr {
			t.Fatalf("Get Flaky Tests: got err %v, want err %v", gotErr, test.wantErr)
		}
		if test.job.flakyReports == nil {
			t.Fatalf("Get Flaky Tests: did not populate job cache")
		}
	}
}

func testGetNonFlakyTests(t *testing.T) {
	cases := []struct {
		failed, flaky, want []string
	}{
		{[]string{}, fakeFlakyTests, []string{}},                                    // no failed tests
		{fakeFlakyTests, []string{}, fakeFlakyTests},                                // no flaky tests
		{fakeFlakyTests, fakeFlakyTests, []string{}},                                // equal
		{[]string{"test0", "extraFailed"}, fakeFlakyTests, []string{"extraFailed"}}, // failed but not flaky
	}
	for _, test := range cases {
		got := getNonFlakyTests(test.failed, test.flaky)
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("Get Non Flaky Tests: got %v, want %v", got, test.want)
		}
	}
}
