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

// This is a really simple image for the kourier integration tests.
// Authorizes requests with the path "$PATH_PREFIX/success" denies all the others.

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

var pathPrefix = os.Getenv("PATH_PREFIX")

func check(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == fmt.Sprintf("%s/success", pathPrefix) || req.URL.Path == fmt.Sprintf("%s/healthz", pathPrefix) {
		log.Print("TRUE")
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Print("FAIL")
	w.WriteHeader(http.StatusForbidden)
}

func main() {
	http.HandleFunc(fmt.Sprintf("%s/", pathPrefix), check)

	log.Printf("Running the External Authz service.")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
