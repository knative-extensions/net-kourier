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

package calc

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"knative.dev/test-infra/tools/coverage/githubUtil"
	"knative.dev/test-infra/tools/coverage/str"
)

type Incremental struct {
	base Coverage
	new  Coverage
}

func (inc Incremental) delta() float32 {
	baseRatio, _ := inc.base.Ratio()
	newRatio, _ := inc.new.Ratio()
	return newRatio - baseRatio
}

func (inc Incremental) deltaForCovbot() string {
	if inc.base.nAllStmts == 0 {
		return ""
	}
	return str.PercentageForCovbotDelta(inc.delta())
}

func (inc Incremental) oldCovForCovbot() string {
	if inc.base.nAllStmts == 0 {
		return "Do not exist"
	}
	return inc.base.Percentage()
}

func (inc Incremental) String() string {
	return fmt.Sprintf("<%s> (%d / %d) %s ->(%d / %d) %s", inc.base.Name(),
		inc.base.nCoveredStmts, inc.base.nAllStmts, inc.base.Percentage(),
		inc.new.nCoveredStmts, inc.new.nAllStmts, inc.new.Percentage())
}

type GroupChanges struct {
	Added     []Coverage
	Deleted   []Coverage
	Unchanged []Coverage
	Changed   []Incremental
	BaseGroup *CoverageList
	NewGroup  *CoverageList
}

func sorted(m map[string]Coverage) (result []Coverage) {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		result = append(result, m[k])
	}
	return
}

// NewGroupChanges compares the newList of coverage against the base list and
// returns the result
func NewGroupChanges(baseList *CoverageList, newList *CoverageList) *GroupChanges {
	var added, unchanged []Coverage
	var changed []Incremental
	baseFilesMap := baseList.Map()
	for _, newCov := range newList.group {
		newCovName := newCov.Name()
		baseCov, ok := baseFilesMap[newCovName]
		isNewFile := false
		if !ok {
			added = append(added, newCov)
			baseCov = *newCoverage(newCovName)
			isNewFile = true
		}

		// after all the deletions, the leftover would be the elements that only exists in base group,
		// in other words, the files that is deleted in the new group
		delete(baseFilesMap, newCovName)

		incremental := Incremental{baseCov, newCov}
		delta := incremental.delta()
		if delta == 0 && !isNewFile {
			unchanged = append(unchanged, newCov)
		} else {
			changed = append(changed, incremental)
		}
	}

	return &GroupChanges{
		Added:     added,
		Deleted:   sorted(baseFilesMap),
		Unchanged: unchanged,
		Changed:   changed,
		BaseGroup: baseList,
		NewGroup:  newList,
	}
}

// processChangedFiles checks each entry in GroupChanges and see if it is
// include in the github commit. If yes, then include that in the covbot report
func (changes *GroupChanges) processChangedFiles(githubFilePaths map[string]bool) (string, bool, bool) {
	log.Printf("\nFinding joining set of changed files from profile[count=%d] & github\n", len(changes.Changed))
	rows := []string{
		"The following is the coverage report on the affected files.",
		fmt.Sprintf("Say `/test %s` to re-run this coverage report", os.Getenv("JOB_NAME")),
		"",
		"File | Old Coverage | New Coverage | Delta",
		"---- |:------------:|:------------:|:-----:",
	}

	isEmpty, isCoverageLow := true, false

	// empty githubFilePaths indicates the workflow is running without a github connection
	noRepoConnection := len(githubFilePaths) == 0
	if noRepoConnection {
		log.Printf("No github connection. Listing each file with a coverage change.")
	}

	for i, inc := range changes.Changed {
		pathFromProfile := githubUtil.FilePathProfileToGithub(inc.base.Name())

		if noRepoConnection {
			fmt.Printf("File with coverage change: %s", pathFromProfile)
		} else {
			fmt.Printf("Checking if this file is in github change list: %s", pathFromProfile)
		}
		if noRepoConnection || githubFilePaths[pathFromProfile] {
			fmt.Printf("\tYes!")
			rows = append(rows, inc.githubBotRow(i, pathFromProfile))
			isEmpty = false

			if inc.new.IsCoverageLow(changes.NewGroup.covThresholdInt) {
				fmt.Printf("\t(Coverage low!)")
				isCoverageLow = true
			}
		} else {
			fmt.Printf("\tNo")
		}
		fmt.Printf("\n")
	}
	fmt.Println("End of Finding joining set of changed files from profile & github")
	rows = append(rows, "")

	return strings.Join(rows, "\n"), isEmpty, isCoverageLow
}

// githubBotRow returns a string as the content of a row covbot posts
func (inc Incremental) githubBotRow(index int, filepath string) string {
	return fmt.Sprintf("%s | %s | %s | %s",
		fmt.Sprintf("[%s](%s)", filepath, inc.new.lineCovLink),
		inc.oldCovForCovbot(),
		inc.new.Percentage(),
		inc.deltaForCovbot())
}

// ContentForGithubPost constructs the message covbot posts
func (changes *GroupChanges) ContentForGithubPost(files map[string]bool) (string, bool, bool) {
	fmt.Printf("\n%d files changed, reported by github:\n", len(files))
	for githubFilePath := range files {
		fmt.Printf("%s\t", githubFilePath)
	}

	fmt.Printf("\n\n")
	return changes.processChangedFiles(files)
}
