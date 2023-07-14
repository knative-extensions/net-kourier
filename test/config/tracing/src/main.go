/*
Copyright 2020 The Knative Authors

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

// This is a simple image for the kourier integration tests.
// Emulates a tracing backend server listening on 9411, just logging incoming requests

import (
	"io"
	"log"
	"net/http"
)

func logRequest(w http.ResponseWriter, req *http.Request) {
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("%s %s - %s", req.Method, req.URL.Path, string(reqBody))

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/", logRequest)

	log.Printf("Running the tracing backend server.")
	//nolint // ignore G114: Use of net/http serve function that has no support for setting timeouts (gosec)
	if err := http.ListenAndServe(":9411", nil); err != nil {
		panic(err)
	}
}
