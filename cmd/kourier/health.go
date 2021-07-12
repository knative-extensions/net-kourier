/*
Copyright 2021 The Knative Authors

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
	"log"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	// connectionFailure indicates connection failed.
	connectionFailure = 2
	// repcFailure indicates rpc failed.
	rpcFailure = 3
	// unhealthy indicates rpc succeeded but indicates unhealthy service.
	unhealthy = 4

	timeout = 100 * time.Millisecond
)

func check(addr string) int {
	dialCtx, dialCancel := context.WithTimeout(context.Background(), timeout)
	defer dialCancel()
	conn, err := grpc.DialContext(dialCtx, addr, grpc.WithBlock(), grpc.WithInsecure())
	if err != nil {
		log.Printf("failed to connect to service at %q: %+v", addr, err)
		return connectionFailure
	}
	defer conn.Close()

	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), timeout)
	defer rpcCancel()
	resp, err := healthpb.NewHealthClient(conn).Check(rpcCtx, &healthpb.HealthCheckRequest{Service: ""})
	if err != nil {
		log.Printf("failed to do health rpc call: %+v", err)
		return rpcFailure
	}

	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		log.Printf("service unhealthy (responded with %q)", resp.GetStatus().String())
		return unhealthy
	}
	log.Printf("status: %v", resp.GetStatus().String())
	return 0
}
