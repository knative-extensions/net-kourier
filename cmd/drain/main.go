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
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	drainTime := os.Getenv("DRAIN_TIME_SECONDS")
	if drainTime == "" {
		drainTime = "15"
	}
	seconds, err := strconv.Atoi(drainTime)
	if err != nil || seconds < 0 {
		log.Fatal("DRAIN_TIME_SECONDS must be a non-negative integer")
	} else if seconds == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()

	err = drain(ctx, "http://127.0.0.1:9901/drain_listeners?graceful")
	if err != nil {
		log.Printf("Failed to drain Envoy: %v", err)
	}

	// Keep the gateway alive for the remainder of the drain window, even if the
	// admin request fails, while Service endpoints are updated.
	<-ctx.Done()
	if err != nil {
		os.Exit(1)
	}
}

func drain(ctx context.Context, url string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected drain response: %s", response.Status)
	}
	return nil
}
