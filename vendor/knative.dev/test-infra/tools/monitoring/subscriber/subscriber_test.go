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

package subscriber

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"cloud.google.com/go/pubsub"
	"knative.dev/test-infra/tools/monitoring/prowapi"
)

type contextKey int

const keyError contextKey = iota

type fakeSubscriber struct{}

func getFakeSubscriber(n string) *Client {
	return &Client{&fakeSubscriber{}}
}

func (fs *fakeSubscriber) Receive(ctx context.Context, f func(context.Context, *pubsub.Message)) error {
	if err := ctx.Value(keyError); err != nil {
		return err.(error)
	}
	return nil
}

func TestSubscriberClient_ReceiveMessageAckAll(t *testing.T) {
	receivedMsgs := make([]*prowapi.ReportMessage, 3)

	type arguments struct {
		ctx context.Context
		f   func(*prowapi.ReportMessage)
	}
	tests := []struct {
		name string
		args arguments
		want error
	}{
		{
			name: "Message Received",
			args: arguments{
				ctx: context.Background(),
				f: func(message *prowapi.ReportMessage) {
					receivedMsgs[0] = message
				},
			},
			want: nil,
		},
		{
			name: "ReceiveError",
			args: arguments{
				ctx: context.WithValue(context.Background(), keyError, errors.New("code = NotFound desc = Resource not found")),
				f: func(message *prowapi.ReportMessage) {
					receivedMsgs[0] = message
				},
			},
			want: errors.New("code = NotFound desc = Resource not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := getFakeSubscriber("fake subscriber")
			if got := fs.ReceiveMessageAckAll(tt.args.ctx, tt.args.f); !isSameError(got, tt.want) {
				t.Errorf("ReceiveMessageAutoAck(%v), got: %v, want: %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestToReportMessage(t *testing.T) {
	tests := []struct {
		name string
		arg  *pubsub.Message
		want *prowapi.ReportMessage
	}{
		{
			name: "Valid report",
			arg: &pubsub.Message{
				Data: []byte(`{"project":"knative-tests","topic":"knative-monitoring","runid":"post-knative-serving-go-coverage-dev","status":"triggered","url":"","gcs_path":"gs://","refs":[{"org":"knative","repo":"serving","base_ref":"master","base_sha":"ce96dd74b1c85f024d63ce0991d4bf61aced582a","clone_uri":"https://github.com/knative/serving.git"}],"job_type":"postsubmit","job_name":"post-knative-serving-go-coverage-dev"}`)},
			want: &prowapi.ReportMessage{
				Project: "knative-tests",
				Topic:   "knative-monitoring",
				RunID:   "post-knative-serving-go-coverage-dev",
				Status:  "triggered",
				URL:     "",
				GCSPath: "gs://",
				Refs: []prowapi.Refs{
					{
						Org:      "knative",
						Repo:     "serving",
						BaseRef:  "master",
						BaseSHA:  "ce96dd74b1c85f024d63ce0991d4bf61aced582a",
						CloneURI: "https://github.com/knative/serving.git",
					},
				},
				JobType: "postsubmit",
				JobName: "post-knative-serving-go-coverage-dev",
			},
		},
		{
			name: "Invalid Report",
			arg: &pubsub.Message{
				Data: []byte(`Random Weird Format`)},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := getFakeSubscriber("fake subscriber")
			if got, _ := fs.toReportMessage(tt.arg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toReportMessage(%v), got: %v, want: %v", tt.arg, got, tt.want)
			}
		})
	}
}

func isSameError(err1 error, err2 error) bool {
	return (err1 == nil && err2 == nil) ||
		(err1 != nil && err2 != nil && err1.Error() == err2.Error())
}
