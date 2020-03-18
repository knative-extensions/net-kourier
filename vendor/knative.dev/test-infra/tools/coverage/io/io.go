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
package io

import (
	"fmt"
	"log"
	"os"
	"path"

	"knative.dev/test-infra/tools/coverage/logUtil"
)

// CreateMarker produces empty file as marker
func CreateMarker(dir, fileName string) {
	Write(nil, dir, fileName)
	log.Printf("Created marker file '%s'\n", fileName)
}

// Write writes the content of the string to a file in the directory
func Write(content *string, destinationDir, fileName string) {
	filePath := path.Join(destinationDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		logUtil.LogFatalf("Error writing file: %v", err)
	} else {
		log.Printf("Created file:%s", filePath)
		if content == nil {
			log.Printf("No content to be written to file '%s'", fileName)
		} else {
			fmt.Fprint(file, *content)
		}
	}
	defer file.Close()
}
