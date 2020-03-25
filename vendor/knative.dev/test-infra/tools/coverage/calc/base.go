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

// base.go stores the main structs and their methods used by the coverage app

package calc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"knative.dev/test-infra/tools/coverage/git"
	"knative.dev/test-infra/tools/coverage/githubUtil"
	"knative.dev/test-infra/tools/coverage/str"
)

type codeBlock struct {
	fileName      string // the file the code block is in
	numStatements int    // number of statements in the code block
	coverageCount int    // number of times the block is covered
}

func (blk *codeBlock) filePathInGithub() string {
	return githubUtil.FilePathProfileToGithub(blk.fileName)
}

// add blk Coverage to file group Coverage
func (blk *codeBlock) addToGroupCov(g *CoverageList) {
	if g.size() == 0 || g.lastElement().Name() != blk.fileName {
		// when a new file name is processed
		coverage := newCoverage(blk.fileName)
		g.append(coverage)
	}
	cov := g.lastElement()
	cov.nAllStmts += blk.numStatements
	if blk.coverageCount > 0 {
		cov.nCoveredStmts += blk.numStatements
	}
}

// Check if the given file is a concerned file. If it is, add it to the concerned files collection and return true
func updateConcernedFiles(concernedFiles map[string]bool, filePath string, isPresubmit bool) bool {
	// get linguist generated attribute value for the file.
	// If true => needs to be skipped for coverage.
	isConcerned, ok := concernedFiles[filePath]
	if ok {
		return true
	}

	// presubmit already have concerned files defined.
	// we don't need to check git attributes here
	if isPresubmit {
		return false
	}

	isConcerned = !git.IsCoverageSkipped(filePath)
	concernedFiles[filePath] = isConcerned
	return isConcerned
}

// convert a line in profile file to a codeBlock struct
func toBlock(line string) (res *codeBlock) {
	slice := strings.Split(line, " ")
	blockName := slice[0]
	nStmts, _ := strconv.Atoi(slice[1])
	coverageCount, _ := strconv.Atoi(slice[2])
	return &codeBlock{
		fileName:      blockName[:strings.Index(blockName, ":")],
		numStatements: nStmts,
		coverageCount: coverageCount,
	}
}

// Coverage stores test coverage summary data for one file
type Coverage struct {
	name          string
	nCoveredStmts int
	nAllStmts     int
	lineCovLink   string
}

func newCoverage(name string) *Coverage {
	return &Coverage{name, 0, 0, ""}
}

// Name returns the file name
func (c *Coverage) Name() string {
	return c.name
}

// Percentage returns the percentage of statements covered
func (c *Coverage) Percentage() string {
	ratio, err := c.Ratio()
	if err == nil {
		return str.PercentStr(ratio)
	}

	return "N/A"
}

// PercentageForTestgrid returns the percentage of statements covered
func (c *Coverage) PercentageForTestgrid() string {
	ratio, err := c.Ratio()
	if err == nil {
		return str.PercentageForTestgrid(ratio)
	}

	return ""
}

func (c *Coverage) Ratio() (ratio float32, err error) {
	if c.nAllStmts == 0 {
		err = fmt.Errorf("[%s] has 0 statement", c.Name())
	} else {
		ratio = float32(c.nCoveredStmts) / float32(c.nAllStmts)
	}
	return
}

// String returns the summary of coverage in string
func (c *Coverage) String() string {
	ratio, err := c.Ratio()
	if err == nil {
		return fmt.Sprintf("[%s]\t%s (%d of %d stmts) covered", c.Name(),
			str.PercentStr(ratio), c.nCoveredStmts, c.nAllStmts)
	}
	return "ratio not exist"
}

func (c *Coverage) LineCovLink() string {
	return c.lineCovLink
}

func (c *Coverage) SetLineCovLink(link string) {
	c.lineCovLink = link
}

// IsCoverageLow checks if the coverage is less than the threshold.
func (c *Coverage) IsCoverageLow(covThresholdInt int) bool {
	covThreshold := float32(covThresholdInt) / 100
	ratio, err := c.Ratio()
	if err == nil {
		return ratio < covThreshold
	}
	// go file with no statement should not be marked as low coverage file
	return false
}

func SortCoverages(cs []Coverage) {
	sort.Slice(cs, func(i, j int) bool {
		return cs[i].Name() < cs[j].Name()
	})
}
