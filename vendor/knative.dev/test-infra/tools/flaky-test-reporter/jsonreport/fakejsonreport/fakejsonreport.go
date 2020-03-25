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

package fakejsonreport

import (
	"encoding/json"
	"fmt"

	"knative.dev/test-infra/tools/flaky-test-reporter/jsonreport"
)

// FakeClient fakes the jsonreport client. All file IO is redirected to data array
type FakeClient struct {
	data []byte
}

// Initialize wraps prow's init, which must be called before any other prow functions are used.
func Initialize(serviceAccount string) (*FakeClient, error) {
	return &FakeClient{}, nil
}

// CreateReport generates a flaky report for a given repository, and optionally
// writes it to disk.
func (c *FakeClient) CreateReport(repo string, flaky []string, writeFile bool) (*jsonreport.Report, error) {
	report := &jsonreport.Report{
		Repo:  repo,
		Flaky: flaky,
	}
	if writeFile {
		data, err := json.Marshal(report)
		if err != nil {
			return nil, err
		}
		c.data = data
	}
	return report, nil
}

// GetFlakyTests gets the latest flaky tests from the given repo
func (c *FakeClient) GetFlakyTests(jobName, repo string) ([]string, error) {
	reports, err := c.GetFlakyTestReport("", repo, -1)
	if err != nil {
		return nil, err
	}
	if len(reports) != 1 {
		return nil, fmt.Errorf("invalid entries for given repo: %d", len(reports))
	}
	return reports[0].Flaky, nil
}

// GetReportRepos gets all of the repositories where we collect flaky tests.
func (c *FakeClient) GetReportRepos(jobName string) ([]string, error) {
	reports, err := c.GetFlakyTestReport("", "", -1)
	if err != nil {
		return nil, err
	}
	var results []string
	for _, r := range reports {
		results = append(results, r.Repo)
	}
	return results, nil
}

// GetFlakyTestReport collects flaky test reports from the given buildID and repo.
// Use repo = "" to get reports from all repositories, and buildID = -1 to get the
// most recent report
func (c *FakeClient) GetFlakyTestReport(jobName, repo string, buildID int) ([]jsonreport.Report, error) {
	report := jsonreport.Report{}
	if err := json.Unmarshal(c.data, &report); err != nil {
		return nil, err
	}
	return []jsonreport.Report{report}, nil
}
