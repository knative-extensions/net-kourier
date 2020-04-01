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

package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"knative.dev/test-infra/pkg/mysql"
)

const alertInsertStmt = `
		INSERT INTO Alerts (Sent, ErrorPattern) VALUES (?,?)
		ON DUPLICATE KEY UPDATE Sent = (?)`

// DB holds an active database connection created in `config`
type DB struct {
	*sql.DB
}

// ErrorLog stores a row in the "ErrorLogs" db table
// Table schema: knative.dev/test-infra/tools/monitoring/mysql/schema.sql
type ErrorLog struct {
	Pattern     string
	Msg         string
	JobName     string
	PRNumber    int
	BuildLogURL string
	TimeStamp   time.Time
}

// Alert maps to the Alerts table
// Table schema: knative.dev/test-infra/tools/monitoring/mysql/schema.sql
type Alert struct {
	ErrorPattern string
	Sent         time.Time
}

// String returns the string representation of the struct used in alert message
func (e ErrorLog) String() string {
	return fmt.Sprintf("[%v] %s (Job: %s, PR: %v, BuildLog: %s)",
		e.TimeStamp, e.Msg, e.JobName, e.PRNumber, e.BuildLogURL)
}

// NewDB returns the DB object with an active database connection
func NewDB(c *mysql.DBConfig) (*DB, error) {
	db, err := c.Connect()
	return &DB{db}, err
}

// AddErrorLog insert a new error to the ErrorLogs table
func (db *DB) AddErrorLog(errPat string, errMsg string, jobName string, prNum int, blogURL string) error {
	stmt, err := db.Prepare(`INSERT INTO ErrorLogs(ErrorPattern, ErrorMsg, JobName, PRNumber, BuildLogURL, TimeStamp)
				VALUES (?, ?, ?, ?, ?, ?)`)
	defer stmt.Close()

	if err != nil {
		return err
	}

	_, err = stmt.Exec(errPat, errMsg, jobName, prNum, blogURL, time.Now())
	return err
}

// ListErrorLogs returns all jobs stored in ErrorLogs table within the time window
func (db *DB) ListErrorLogs(errorPattern string, window time.Duration) ([]ErrorLog, error) {
	var result []ErrorLog

	// the timestamp we want to start collecting logs
	startTime := time.Now().Add(-1 * window)

	rows, err := db.Query(`
		SELECT ErrorPattern, ErrorMsg, JobName, PRNumber, BuildLogURL, TimeStamp
		FROM ErrorLogs
		WHERE ErrorPattern=? and TimeStamp > ?`,
		errorPattern, startTime)

	if err != nil {
		return result, err
	}

	for rows.Next() {
		entry := ErrorLog{}
		err = rows.Scan(&entry.Pattern, &entry.Msg, &entry.JobName, &entry.PRNumber, &entry.BuildLogURL, &entry.TimeStamp)
		if err != nil {
			return result, err
		}
		result = append(result, entry)
	}

	return result, nil
}

// ListAlerts returns all error pattern and timestamps in the Alerts table
func (db *DB) ListAlerts() ([]*Alert, error) {
	rows, err := db.Query(`
        SELECT ErrorPattern, Sent
        FROM Alerts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		a := &Alert{}
		err = rows.Scan(&a.ErrorPattern, &a.Sent)
		if err != nil {
			return nil, fmt.Errorf("mysql: could not read row: %v", err)
		}

		alerts = append(alerts, a)
	}

	return alerts, nil
}

// AddAlert inserts a new error pattern and alert time (now) to Alerts table
// If the pattern already exists, update the alert time
func (db *DB) AddAlert(errorPattern string) error {
	now := time.Now()
	_, err := db.Query(alertInsertStmt, now, errorPattern, now)
	return err
}

// DeleteAlert deletes a row (alert) from the Alerts table
func (db *DB) DeleteAlert(errorPattern string) error {
	stmt, err := db.Prepare(`
				DELETE FROM Alerts
				WHERE ErrorPattern = ?`)
	defer stmt.Close()

	if err == nil {
		err = execAffectingOneRow(stmt, errorPattern)
	}

	return err
}

// IsFreshAlertPattern checks the Alerts table to see if the error pattern hasn't been alerted within the time window
func (db *DB) IsFreshAlertPattern(errorPattern string, window time.Duration) (bool, error) {
	var id int
	var sent time.Time

	row := db.QueryRow(`
		SELECT ID, Sent
		FROM Alerts
		WHERE ErrorPattern = ?`,
		errorPattern)

	if err := row.Scan(&id, &sent); err != nil {
		if err != sql.ErrNoRows {
			return false, err
		}

		// if no record found
		return true, nil
	}

	if sent.Add(window).Before(time.Now()) {
		// if previous alert expires.
		log.Printf("previous alert timestamp=%v expired, alert window size=%v", sent, window)
		return true, nil
	}

	log.Printf("previous alert not expired (timestamp=%v), "+
		"alert window size=%v, no alert will be sent", sent, window)
	return false, nil
}

// IsPatternAlerting checks whether the given error pattern meets the alert condition
func (db *DB) IsPatternAlerting(errorPattern, jobPattern string, window time.Duration, aTotal, aJobs, aPRs int) (bool, error) {
	var nMatches, nJobs, nPRs int
	// the timestamp we want to start collecting logs
	startTime := time.Now().Add(-1 * window)

	row := db.QueryRow(`
		SELECT
			COUNT(*),
			COUNT(DISTINCT JobName),
			COUNT(DISTINCT PrNumber)
		FROM ErrorLogs
		WHERE ErrorPattern = ?
		AND JobName REGEXP ?
		AND TimeStamp > ?`, errorPattern, jobPattern, startTime)

	err := row.Scan(&nMatches, &nJobs, &nPRs)
	return err == nil && nMatches >= aTotal && nJobs >= aJobs && nPRs >= aPRs, err
}

// execAffectingOneRow executes a given statement, expecting one row to be affected.
func execAffectingOneRow(stmt *sql.Stmt, args ...interface{}) error {
	r, err := stmt.Exec(args...)
	if err != nil {
		return fmt.Errorf("could not execute statement: %v", err)
	}
	if rowsAffected, err := r.RowsAffected(); err != nil {
		return fmt.Errorf("could not get rows affected: %v", err)
	} else if rowsAffected != 1 {
		return fmt.Errorf("expected 1 row affected, got %d", rowsAffected)
	}
	return nil
}
