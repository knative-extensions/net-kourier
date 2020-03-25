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
	"database/sql/driver"
	"fmt"
	"io/ioutil"

	_ "github.com/go-sql-driver/mysql"
)

const driverName = "mysql"

// DBConfig is the configuration used to connection to database
type DBConfig struct {
	Username     string
	Password     string
	Host         string
	Port         string
	DatabaseName string
}

// ConfigureDB reads the database user name and password from a file (secret) and creates a DBConfig
// that can be used to connect to the database.
func ConfigureDB(userSecret, passSecret, hostSecret, dbPort, dbName string) (*DBConfig, error) {
	user, err := ioutil.ReadFile(userSecret)
	if err != nil {
		return nil, err
	}

	pass, err := ioutil.ReadFile(passSecret)
	if err != nil {
		return nil, err
	}

	host, err := ioutil.ReadFile(hostSecret)
	if err != nil {
		return nil, err
	}

	config := DBConfig{
		Username:     string(user),
		Password:     string(pass),
		Host:         string(host),
		Port:         dbPort,
		DatabaseName: dbName,
	}

	return &config, nil
}

func (c DBConfig) TestConn() error {
	conn, err := c.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	return nil
}

func (c DBConfig) Connect() (*sql.DB, error) {
	// adds "?parseTime=true" so that TimeStamp field can be scanned as time.Time
	conn, err := sql.Open(driverName, c.dataStoreName(c.DatabaseName)+"?parseTime=true")
	if err != nil {
		return nil, fmt.Errorf("could not get a connection: %v", err)
	}

	if conn.Ping() == driver.ErrBadConn {
		return nil, fmt.Errorf("could not connect to the datastore. " +
			"could be bad address, or this address is inaccessible from your host.\n")
	}

	return conn, nil
}

func (c DBConfig) dataStoreName(dbName string) string {
	var cred string
	// [username[:password]@]
	if len(c.Username) > 0 {
		cred = c.Username
		if len(c.Password) > 0 {
			cred = cred + ":" + c.Password
		}
		cred = cred + "@"
	}

	return fmt.Sprintf("%stcp([%s]:%s)/%s", cred, c.Host, c.Port, dbName)
}

// RollbackTx will try to rollback the transaction and return an error message accordingly
func RollbackTx(tx *sql.Tx, err error) error {
	if rbErr := tx.Rollback(); rbErr != nil {
		return fmt.Errorf("Statement execution failed: %v; rollback failed: %v", err, rbErr)
	}
	return fmt.Errorf("Statement execution failed: %v; rolled back.", err)
}
