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
	"log"
	"regexp"

	"knative.dev/test-infra/tools/monitoring/config"
	"knative.dev/test-infra/tools/monitoring/mysql"
)

// collectMatches collects error messages that matches the patterns from text.
func collectMatches(regexps []regexp.Regexp, text []byte) []mysql.ErrorLog {
	var errorLogs []mysql.ErrorLog
	for _, r := range regexps {
		found := r.Find(text)
		if found != nil {
			errorLogs = append(errorLogs, mysql.ErrorLog{
				Pattern: r.String(),
				Msg:     string(found),
			})
		}
	}
	return errorLogs
}

// ParseLog checks content against given error patterns. Return
// all found error patterns and error messages in pairs.
func ParseLog(content []byte, patterns []string) ([]mysql.ErrorLog, error) {
	regexps, badPatterns := config.CompilePatterns(patterns)
	if len(badPatterns) != 0 {
		log.Printf("The following patterns cannot be compiled: %v", badPatterns)
	}

	return collectMatches(regexps, content), nil
}
