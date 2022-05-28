/*
Copyright 2021 The Knative Authors

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
	"log"
	"net/http"
	"os"

	"knative.dev/networking/pkg/http/probe"
	"knative.dev/networking/test"
)

var retries = 0

func handler(w http.ResponseWriter, r *http.Request) {
	if retries == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	fmt.Fprintf(w, "Retry %d", retries)
	retries++
}

func main() {
	h := probe.NewHandler(http.HandlerFunc(handler))
	port := os.Getenv("PORT")
	if cert, key := os.Getenv("CERT"), os.Getenv("KEY"); cert != "" && key != "" {
		log.Print("Server starting on port with TLS ", port)
		test.ListenAndServeTLSGracefully(cert, key, ":"+port, h.ServeHTTP)
	} else {
		log.Print("Server starting on port ", port)
		test.ListenAndServeGracefully(":"+port, h.ServeHTTP)
	}
}
