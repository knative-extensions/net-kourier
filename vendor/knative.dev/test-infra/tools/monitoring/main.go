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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"knative.dev/test-infra/pkg/gcs"
	"knative.dev/test-infra/pkg/mysql"
	"knative.dev/test-infra/tools/monitoring/alert"
	"knative.dev/test-infra/tools/monitoring/mail"
	msql "knative.dev/test-infra/tools/monitoring/mysql"
	"knative.dev/test-infra/tools/monitoring/subscriber"
)

var (
	dbConfig    *mysql.DBConfig
	mailConfig  *mail.Config
	client      *subscriber.Client
	alertClient *alert.Client
	db          *msql.DB
)

func main() {
	var err error

	dbName := flag.String("database-name", "monitoring", "The monitoring database name")
	dbPort := flag.String("database-port", "3306", "The monitoring database port")

	dbUserSF := flag.String("database-user", "/secrets/cloudsql/monitoringdb/username", "Database user secret file")
	dbPassSF := flag.String("database-password", "/secrets/cloudsql/monitoringdb/password", "Database password secret file")
	dbHost := flag.String("database-host", "/secrets/cloudsql/monitoringdb/host", "Database host secret file")
	mailAddrSF := flag.String("sender-email", "/secrets/sender-email/mail", "Alert sender email address file")
	mailPassSF := flag.String("sender-password", "/secrets/sender-email/password", "Alert sender email password file")

	serviceAccount := flag.String("service-account", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"), "JSON key file for GCS service account")

	flag.Parse()

	dbConfig, err = mysql.ConfigureDB(*dbUserSF, *dbPassSF, *dbHost, *dbPort, *dbName)
	if err != nil {
		log.Fatal(err)
	}

	db, err = msql.NewDB(dbConfig)
	if err != nil {
		log.Fatal(err)
	}

	mailConfig, err = mail.NewMailConfig(*mailAddrSF, *mailPassSF)
	if err != nil {
		log.Fatal(err)
	}

	err = gcs.Authenticate(context.Background(), *serviceAccount)
	if err != nil {
		log.Fatalf("Failed to authenticate gcs %+v", err)
	}

	alertClient, err = alert.Setup(db, mailConfig)
	if err != nil {
		log.Fatalf("Failed to setup test-infra monitoring: %v\n", err)
	}
	alertClient.RunAlerting()

	// use PORT environment variable, or default to 8080
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}

	// register hello function to handle all requests
	server := http.NewServeMux()
	server.HandleFunc("/test-conn", testCloudSQLConn)
	server.HandleFunc("/send-mail", sendTestEmail)
	server.HandleFunc("/test-insert", testInsert)

	// start the web server on port and accept requests
	log.Printf("Server listening on port %s", port)
	err = http.ListenAndServe(":"+port, server)
	log.Fatal(err)
}

func testCloudSQLConn(w http.ResponseWriter, r *http.Request) {
	log.Printf("Serving request: %s", r.URL.Path)
	fmt.Fprintf(w, "Testing mysql database connection...")

	_, err := dbConfig.Connect()
	if err != nil {
		fmt.Fprintf(w, "Failed to ping the database %v", err)
		return
	}
}

func sendTestEmail(w http.ResponseWriter, r *http.Request) {
	log.Printf("Serving request: %s", r.URL.Path)
	log.Println("Sending test email")

	recipients, ok := r.URL.Query()["recipient"]
	if !ok || len(recipients[0]) < 1 {
		fmt.Fprintln(w, "Url Param 'recipient' is missing")
		return
	}

	err := mailConfig.Send(
		recipients,
		"Test Subject",
		"Test Content",
	)
	if err != nil {
		fmt.Fprintf(w, "Failed to send email %v", err)
		return
	}

	fmt.Fprintln(w, "Sent the Email")
}

func testInsert(w http.ResponseWriter, r *http.Request) {
	log.Printf("Serving request: %s", r.URL.Path)
	log.Println("testing insert to database")

	err := db.AddErrorLog("test error pattern", "test err message", "test job", 1, "gs://")
	if err != nil {
		fmt.Fprintf(w, "Failed to insert to database: %+v\n", err)
		return
	}

	fmt.Fprintln(w, "Success")
}
