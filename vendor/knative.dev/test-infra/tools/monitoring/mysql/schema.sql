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

# These tables only need to be created once,
# and they should be created before any run of the application

CREATE TABLE ErrorLogs
(
	ID           int           NOT NULL AUTO_INCREMENT,
	ErrorPattern varchar(4095) NOT NULL,
	ErrorMsg     varchar(4095) NOT NULL,
	JobName      varchar(1023) NOT NULL, /*e.g. pull-knative-serving-integration-tests*/
	PRNumber     int, /*pull request number; null for non pull jobs*/
	BuildLogURL  varchar(1023) NOT NULL, /*link to build-log.txt file*/
	TimeStamp    timestamp, /* stamps the time the record is added*/
	PRIMARY KEY (ID)
);

CREATE TABLE Alerts
(
	ID           int           NOT NULL AUTO_INCREMENT,
	ErrorPattern varchar(4095) NOT NULL UNIQUE,
	Sent         timestamp,
	PRIMARY KEY (ID)
)
