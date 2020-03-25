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
	"encoding/json"
	"log"

	"cloud.google.com/go/pubsub"
	"knative.dev/test-infra/tools/monitoring/prowapi"
)

const projectID = "knative-tests"

// pubsub.Client is scoped to a single GCP project. Reuse the pubsub.Client as needed.
var pubsubClient *pubsub.Client

// Client is a wrapper on the subscriber Operation
type Client struct {
	Operation
}

// Operation defines a list of methods for subscribing messages
type Operation interface {
	Receive(ctx context.Context, f func(context.Context, *pubsub.Message)) error
}

// NewSubscriberClient returns a new SubscriberClient used to read crier pubsub messages
func NewSubscriberClient(subName string) (*Client, error) {
	var err error
	if pubsubClient == nil {
		log.Println("pubsub.Client not created yet. Creating the client.")
		if pubsubClient, err = pubsub.NewClient(context.Background(), projectID); err != nil {
			return nil, err
		}
	}
	return &Client{pubsubClient.Subscription(subName)}, nil
}

// ReceiveMessageAckAll acknowledges all incoming pusub messages and convert the pubsub message to ReportMessage.
// It executes `f` only if the pubsub message can be converted to ReportMessage. Otherwise, ignore the message.
func (c *Client) ReceiveMessageAckAll(ctx context.Context, f func(*prowapi.ReportMessage)) error {
	return c.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		if rmsg, err := c.toReportMessage(msg); err != nil {
			log.Printf("Cannot convert pubsub message (%v) to Report message %v", msg, err)
		} else if rmsg != nil {
			f(rmsg)
		}
		msg.Ack()
		log.Printf("Message acked: %q", msg.ID)
	})
}

func (c *Client) toReportMessage(msg *pubsub.Message) (*prowapi.ReportMessage, error) {
	rmsg := &prowapi.ReportMessage{}
	if err := json.Unmarshal(msg.Data, rmsg); err != nil {
		return nil, err
	}
	return rmsg, nil
}
