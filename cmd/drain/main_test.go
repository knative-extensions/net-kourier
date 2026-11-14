/*
Copyright 2026 The Knative Authors

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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDrain(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{name: "success", status: http.StatusOK},
		{name: "admin error", status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/drain_listeners" || !r.URL.Query().Has("graceful") {
					t.Errorf("unexpected drain request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			err := drain(context.Background(), server.URL+"/drain_listeners?graceful")
			if (err != nil) != (tc.status != http.StatusOK) {
				t.Errorf("drain() error = %v, response status = %d", err, tc.status)
			}
		})
	}
}
