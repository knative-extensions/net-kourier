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

package mail

import (
	"reflect"
	"testing"
)

func TestBuildMessage(t *testing.T) {
	type arguments struct {
		sender     string
		recipients []string
		subject    string
		body       string
	}
	tests := []struct {
		name string
		args arguments
		want []byte
	}{
		{
			name: "single recipient",
			args: arguments{
				sender:     "sender",
				recipients: []string{"single@mail.com"},
				subject:    "subject",
				body:       "body",
			},
			want: []byte("From: sender\nTo: single@mail.com\nSubject: subject\n\nbody"),
		},
		{
			name: "multiple recipients",
			args: arguments{
				sender:     "sender",
				recipients: []string{"one@testdomain.com", "two@testdomain.com"},
				subject:    "subject",
				body:       "body",
			},
			want: []byte("From: sender\nTo: one@testdomain.com;two@testdomain.com\nSubject: subject\n\nbody"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{}
			if got := c.buildMessage(tt.args.sender, tt.args.recipients, tt.args.subject, tt.args.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unexpected email message: got %v, want %v", got, tt.want)
			}
		})
	}
}
