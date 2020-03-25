// +build library

/*
Copyright 2018 The Knative Authors

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

package smoke

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"
)

// signal sends a UNIX signal to the test process.
func signal(s os.Signal) {
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(s)
	// Sleep so test won't finish and signal will be received.
	time.Sleep(999)
}

func TestSucceeds(t *testing.T) {
	// Always succeed.
}

func TestFails(t *testing.T) {
	t.Fail()
}

func TestFailsWithFatal(t *testing.T) {
	// Simulate a zap.Fatal() call.
	fmt.Println("fatal\tTestFailsWithFatal\tsimple_test.go:999\tFailed with logger.Fatal()")
	signal(os.Interrupt)
}

func TestFailsWithPanic(t *testing.T) {
	// Simulate a "panic" stack trace.
	fmt.Println("panic: test timed out after 5m0s")
	signal(os.Interrupt)
}

func TestFailsWithSigQuit(t *testing.T) {
	signal(syscall.SIGQUIT)
}
