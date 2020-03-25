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
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

// errorMismatch compares two errors by their error string
func errorMismatch(got error, want error) string {
	exp := "<no error>"
	act := "<no error>"
	if want != nil {
		exp = want.Error()
	}
	if got != nil {
		act = got.Error()
	}
	if strings.Contains(act, exp) {
		return ""
	}
	return fmt.Sprintf("got: '%v'\nwant: '%v'", act, exp)
}

func TestSelectProjects(t *testing.T) {
	datas := []struct {
		projectFlag  string
		yamlFileFlag string
		regexFlag    string
		exp          []string
		err          error
	}{
		{ // Project provided.
			"foo",
			"",
			"",
			[]string{"foo"},
			nil,
		},
		{ // File provided.
			"",
			"testdata/resources.yaml",
			"knative-boskos-.*",
			[]string{"knative-boskos-01", "knative-boskos-02"},
			nil,
		},
		{ // Bad file provided.
			"",
			"/foobar_resources.yamlfoo",
			"",
			[]string{},
			errors.New("no such file or directory"),
		},
		{ // Empty file provided.
			"",
			"testdata/empty.yaml",
			".*",
			[]string{},
			errors.New("no project found"),
		},
		{ // Bad regex provided.
			"",
			"testdata/resources.yaml",
			"--->}][{<---",
			[]string{},
			errors.New("invalid character class range"),
		},
		{ // Unmatching regex provided.
			"",
			"testdata/resources.yaml",
			"foobar-[0-9]",
			[]string{},
			errors.New("no project found"),
		},
	}
	for _, data := range datas {
		r, err := selectProjects(data.projectFlag, data.yamlFileFlag, data.regexFlag)
		errMsg := fmt.Sprintf("Select projects for %q/%q/%q: ", data.projectFlag, data.yamlFileFlag, data.regexFlag)
		if m := errorMismatch(err, data.err); m != "" {
			t.Errorf("%s%s", errMsg, m)
		}
		if data.err != nil {
			continue
		}
		if dif := cmp.Diff(data.exp, r); dif != "" {
			t.Errorf("%sgot(+) is different from wanted(-)\n%v", errMsg, dif)
		}
	}
}

func TestBaseResourceDeleter(t *testing.T) {
	datas := []struct {
		projects []string
		expErr   int
	}{
		{ // No projects.
			[]string{},
			0,
		},
		{ // With less than 10 projects.
			[]string{"p1", "p2"},
			1,
		},
		{ // With more than 10 projects.
			[]string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9", "pA", "pB", "pC", "pD"},
			1,
		},
		{ // With projects, but errors.
			[]string{"p1e", "p2e", "p3e"},
			1,
		},
	}

	for _, data := range datas {
		brd := NewBaseResourceDeleter(data.projects)
		errMsg := fmt.Sprintf("Delete projects %q: ", data.projects)
		if dif := cmp.Diff(data.projects, brd.Projects()); dif != "" {
			t.Errorf("%sgot(+) is different from wanted(-)\n%v", errMsg, dif)
		}
		c, errors := brd.Delete(0, 5, true)
		brd.ShowStats(c, errors)
		if c != 0 {
			t.Errorf("%sgot %d items deleted, wanted 0", errMsg, c)
		}
		if len(errors) != data.expErr {
			t.Errorf("%sgot %d errors, wanted %d", errMsg, len(errors), data.expErr)
		}
		if data.expErr == 1 {
			if errors[0] != "not implemented" {
				t.Errorf("%s got error '%s', wanted 'not implemented'", errMsg, errors[0])
			}
		}
	}
}

func TestDelete(t *testing.T) {
	datas := []struct {
		projects []string
		hours    int
		fn       func(project string, hoursToKeepResource int, dryRun bool) (int, error)
		expCount int
		expErr   []string
	}{
		{ // No projects.
			[]string{},
			9999,
			func(project string, hoursToKeepResource int, dryRun bool) (int, error) {
				t.Error("delete function should not be called")
				return 0, nil
			},
			0,
			[]string{},
		},
		{ // With less than 10 projects.
			[]string{"p1", "p2"},
			0,
			func(project string, hoursToKeepResource int, dryRun bool) (int, error) {
				return 1, nil
			},
			2,
			[]string{},
		},
		{ // With more than 10 projects.
			[]string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9", "pA", "pB", "pC", "pD"},
			0,
			func(project string, hoursToKeepResource int, dryRun bool) (int, error) {
				return 1, nil
			},
			13,
			[]string{},
		},
		{ // With projects, but errors.
			[]string{"p1e", "p2e", "p3e"},
			0,
			func(project string, hoursToKeepResource int, dryRun bool) (int, error) {
				return 1, errors.New("error")
			},
			-1, // There are errors, returned count might vary, ignore.
			[]string{"error"},
		},
		{ // Keep projects newer than 10 hours.
			[]string{"p5", "p7", "p10", "p11", "p12"}, // project number is its age in hours
			10,
			func(project string, hoursToKeepResource int, dryRun bool) (int, error) {
				if n, _ := strconv.Atoi(string(project[1:])); n < hoursToKeepResource {
					return 0, nil
				}
				return 1, nil
			},
			3,
			[]string{},
		},
	}
	for _, data := range datas {
		brd := NewBaseResourceDeleter(data.projects)
		brd.deleteResourceFunc = data.fn
		c, err := brd.Delete(data.hours, 5, true)
		errMsg := fmt.Sprintf("Delete projects %q if older than %d hours: ", data.projects, data.hours)
		if c != data.expCount && data.expCount > -1 {
			t.Errorf("%sgot %d, wanted %d", errMsg, c, data.expCount)
		}
		if dif := cmp.Diff(data.expErr, err); dif != "" {
			t.Errorf("%sgot(+) is different from wanted(-)\n%v", errMsg, dif)
		}
	}

	// Test that dryRun is correctly passed down.
	brd := NewBaseResourceDeleter([]string{"p1"})
	expectedDryRun := true
	brd.deleteResourceFunc = func(project string, hoursToKeepResource int, dryRun bool) (int, error) {
		if dryRun != expectedDryRun {
			t.Errorf("Test that dryRun is correctly passed down, got dryRun=%v, wanted dryRun=%v", dryRun, expectedDryRun)
		}
		return 0, nil
	}
	for _, dr := range []bool{true, false} {
		expectedDryRun = dr
		brd.Delete(0, 1, dr)
	}

}

func TestNewImageDeleter(t *testing.T) {
	datas := []struct {
		sa  string
		err error
	}{
		{ // No service account.
			"",
			nil,
		},
		{ // Bad service account file.
			"/boot/foo/bar/nonono",
			errors.New("No such file or directory"),
		},
		// TODO: Test with a valid service account file.
	}

	projects := []string{"a", "b", "c"}
	registry := "fake_registry"
	for _, data := range datas {
		d, err := NewImageDeleter(projects, registry, data.sa)
		errMsg := fmt.Sprintf("NewImageDeleter with service account %q: ", data.sa)
		if dif := cmp.Diff(projects, d.Projects()); dif != "" {
			t.Errorf("%sgot(+) is different from wanted(-)\n%v", errMsg, dif)
		}
		if dif := cmp.Diff(registry, d.registry); dif != "" {
			t.Errorf("%sgot(+) is different from wanted(-)\n%v", errMsg, dif)
		}
		if m := errorMismatch(err, data.err); m != "" {
			t.Errorf("%s%s", errMsg, m)
		}
	}
}

func TestNewGkeClusterDeleter(t *testing.T) {
	datas := []struct {
		sa  string
		err error
	}{
		{ // No service account.
			"",
			nil,
		},
		{ // Bad service account file.
			"/boot/foo/bar/nonono",
			errors.New("no such file or directory"),
		},
		// TODO: Test with a valid service account file.
	}

	projects := []string{"a", "b", "c"}
	for _, data := range datas {
		d, err := NewGkeClusterDeleter(projects, data.sa)
		errMsg := fmt.Sprintf("NewGkeClusterDeleter with service account %q: ", data.sa)
		if dif := cmp.Diff(projects, d.Projects()); dif != "" {
			t.Errorf("%sgot(+) is different from wanted(-)\n%v", errMsg, dif)
		}
		if m := errorMismatch(err, data.err); m != "" {
			t.Errorf("%s%s", errMsg, m)
		}
	}
}

/*
  TODO: Increase test coverage.

  ImageDeleter.deleteImage (requires mocking remote)
  - bad reference
  - error deleting image
  - delete image

  GkeClusterDeleter.DeleteResources (requires mocking gkeClient)
  - bad project
  - error listing clusters
  - bad timestamp
  - delete cluster (dry run)
  - delete cluster
  - error deleting cluster
  - dry run

  ImageDeleter.DeleteResources (requires mocking name, google)
  - bad project
  - bad registry
  - error walking down registry
  - no images to delete
  - error deleting tag
  - error deleting image
  - dry run

  cleanup (requires mocking deleters)
  - bad --gcr flag
  - error selecting projects
  - error creating deleters
  - skipping deleting images
  - skipping deleting clusters
  - deleting images and clusters
*/
