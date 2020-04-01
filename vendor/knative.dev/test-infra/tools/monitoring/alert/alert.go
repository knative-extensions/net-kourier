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

package alert

import (
	"context"
	"log"

	"knative.dev/test-infra/pkg/gcs"
	"knative.dev/test-infra/tools/monitoring/config"
	"knative.dev/test-infra/tools/monitoring/log_parser"
	"knative.dev/test-infra/tools/monitoring/mail"
	"knative.dev/test-infra/tools/monitoring/mysql"
	"knative.dev/test-infra/tools/monitoring/prowapi"
	"knative.dev/test-infra/tools/monitoring/subscriber"
)

const subName = "test-infra-monitoring-sub"

var alertEmailRecipients = []string{"prime-engprod-sea@google.com"}

// Client holds all the resources required to run alerting
type Client struct {
	*subscriber.Client
	*mail.Config
	db *mysql.DB
}

// Setup sets up the client required to run alerting workflow
func Setup(db *mysql.DB, mc *mail.Config) (*Client, error) {
	sub, err := subscriber.NewSubscriberClient(subName)
	return &Client{sub, mc, db}, err
}

// RunAlerting start the alerting workflow
func (c *Client) RunAlerting() {
	log.Println("Starting alerting workflow")
	go func() {
		err := c.ReceiveMessageAckAll(context.Background(), c.handleReportMessage)
		if err != nil {
			log.Printf("Failed to retrieve messages due to %v", err)
		}
	}()
}

func (c *Client) handleReportMessage(rmsg *prowapi.ReportMessage) {
	if rmsg.Status == prowapi.FailureState || rmsg.Status == prowapi.AbortedState {
		log.Printf("Received Pubsub message in %s state: %v\n", rmsg.Status, rmsg)

		config, err := config.ParseDefaultConfig()
		if err != nil {
			log.Printf("Failed to config yaml (%v): %v\n", config, err)
			return
		}

		blPath, err := gcs.BuildLogPath(rmsg.GCSPath)
		if err != nil {
			log.Printf("Failed to construct build log url from gcs path %s. Error: %v\n", rmsg.GCSPath, err)
			return
		}
		buildLog, err := gcs.ReadURL(context.Background(), blPath)
		if err != nil {
			log.Printf("Failed to read from url %s. Error: %v\n", blPath, err)
			return
		}

		errorLogs, err := log_parser.ParseLog(buildLog, config.CollectErrorPatterns())
		if err != nil {
			log.Printf("Failed to parse content %v. Error: %v\n", string(buildLog), err)
			return
		}
		log.Printf("Parsed errorLogs: %v\n", errorLogs)

		for _, el := range errorLogs {
			c.handleSingleError(config, rmsg, &el)
		}
	}
}

func (c *Client) handleSingleError(config *config.Config, rmsg *prowapi.ReportMessage, el *mysql.ErrorLog) {
	var err error

	// Add the PR number if it is a pull request job
	log.Println("Adding Error Log to the table")
	if len(rmsg.Refs) <= 0 || len(rmsg.Refs[0].Pulls) <= 0 {
		err = c.db.AddErrorLog(el.Pattern, el.Msg, rmsg.JobName, 0, rmsg.URL)
	} else {
		err = c.db.AddErrorLog(el.Pattern, el.Msg, rmsg.JobName, rmsg.Refs[0].Pulls[0].Number, rmsg.URL)
	}
	if err != nil {
		log.Printf("Failed to insert error to db %+v\n", err)
		return
	}

	log.Println("Selecting the config")
	sc, noMatchErr := config.Select(el.Pattern, rmsg.JobName)
	if noMatchErr != nil {
		log.Printf("No matching config found for pattern (%v) job(%v): %v", el.Pattern, rmsg.JobName, noMatchErr)
		return
	}

	log.Println("Sending the alert")
	_, err = c.Alert(el.Pattern, sc, c.db)
	if err != nil {
		log.Printf("Failed to Alert %v", err)
	}
}

// Alert checks alert condition and alerts table and send alert mail conditionally
func (c *Client) Alert(errorPattern string, s *config.SelectedConfig, db *mysql.DB) (bool, error) {
	log.Println("Fetching error logs")
	errorLogs, err := db.ListErrorLogs(errorPattern, s.Duration())
	if err != nil {
		return false, err
	}

	log.Println("Building Report and checking alert conditions")
	report := newReport(errorLogs)
	if !report.CheckAlertCondition(s) {
		return false, nil
	}

	log.Println("checking if the alert is a fresh alert pattern")
	ok, err := db.IsFreshAlertPattern(errorPattern, s.Duration())
	if err != nil || !ok {
		return false, err
	}

	log.Println("Adding the new alert pattern to the database")
	if err := db.AddAlert(errorPattern); err != nil {
		return false, err
	}

	log.Println("Generating and sending the alert email")
	mcont := mailContent{*report, errorPattern, s.Hint, s.Duration()}
	err = c.Send(alertEmailRecipients, mcont.subject(), mcont.body())
	return err == nil, err
}
