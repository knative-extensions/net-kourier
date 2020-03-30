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

// file.go contains help functions for updating files locally with provided versions struct

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"knative.dev/pkg/test/helpers"

	"knative.dev/test-infra/pkg/common"
)

// Update all tags in a byte slice
func (pv *PRVersions) updateAllTags(content []byte, imageFilter *regexp.Regexp) ([]byte, string, []string) {
	var msg string
	var msgs []string
	indexes := imageFilter.FindAllSubmatchIndex(content, -1)
	// Not finding any images is not an error.
	if indexes == nil {
		return content, msg, msgs
	}

	var res string
	lastIndex := 0
	for _, m := range indexes {
		// Append from end of last match to end of image part, including ":"
		res += string(content[lastIndex : m[imageRootPart*2+1]+1])
		// Image part of a version, i.e. the portion before ":"
		image := string(content[m[imageRootPart*2]:m[imageRootPart*2+1]])
		// Tag part of a version, i.e. the portion after ":"
		tag := string(content[m[imageTagPart*2]:m[imageTagPart*2+1]])
		// m[1] is the end index of current match
		lastIndex = m[1]

		iv := pv.getIndex(image, tag)
		if pv.images[image][iv].newVersion != "" {
			res += pv.images[image][iv].newVersion
			msg += fmt.Sprintf("\nImage: %s\nOld Tag: %s\nNew Tag: %s", image, tag, pv.images[image][iv].newVersion)
		} else {
			tmp := fmt.Sprintf("There's no new version for image %s, keeping version: '%s:%s'.\n", image, image, tag)
			log.Println(tmp)
			msgs = append(msgs, tmp)
			res += tag
		}
	}
	res += string(content[lastIndex:])

	return []byte(res), msg, msgs
}

// UpdateFile updates a file in place.
func (pv *PRVersions) updateFile(filename string, imageFilter *regexp.Regexp, dryrun bool) ([]string, error) {
	var msgs []string
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return msgs, fmt.Errorf("failed to read %s: %v", filename, err)
	}

	newContent, msg, msgs := pv.updateAllTags(content, imageFilter)
	if err := helpers.Run(
		fmt.Sprintf("Update file '%s':%s", filename, msg),
		func() error {
			return ioutil.WriteFile(filename, newContent, 0644)
		},
		dryrun,
	); err != nil {
		return msgs, fmt.Errorf("failed to write %s: %v", filename, err)
	}
	return msgs, nil
}

// Walk through all files, and update all tags
func (pv *PRVersions) updateAllFiles(fileFilters []*regexp.Regexp, imageFilter *regexp.Regexp,
	dryrun bool) ([]string, error) {
	var msgs []string
	if err := common.CDToRootDir(); err != nil {
		return msgs, fmt.Errorf("failed to change to root dir")
	}

	var errs []error
	for _, p := range configPaths {
		err := filepath.Walk(p, func(filename string, info os.FileInfo, err error) error {
			for _, ff := range fileFilters {
				if ff.Match([]byte(filename)) {
					tmp, err := pv.updateFile(filename, imageFilter, dryrun)
					msgs = append(msgs, tmp...)
					if err != nil {
						return fmt.Errorf("failed to update path %s '%v'", filename, err)
					}
				}
			}
			return nil
		})
		errs = append(errs, err)
	}
	return msgs, helpers.CombineErrors(errs)
}
