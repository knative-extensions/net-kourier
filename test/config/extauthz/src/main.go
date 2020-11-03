/*
Copyright 2020 The Knative Authors

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

// This is a really simple image for the kourier integration tests.
// Authorizes requests with the path "/success" denies all the others.

import (
	"context"
	"log"
	"net"

	authZ "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	"github.com/golang/protobuf/ptypes/any"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
)

type Auth struct{}

func (ea Auth) Check(ctx context.Context, ar *authZ.CheckRequest) (*authZ.CheckResponse, error) {
	if ar.Attributes.Request.Http.Path == "/success" || ar.Attributes.Request.Http.Path == "/healthz" {
		log.Print("TRUE")
		return &authZ.CheckResponse{
			Status: &status.Status{
				Code: int32(code.Code_OK),
			},
		}, nil
	}

	log.Print("FAIL")
	return &authZ.CheckResponse{
		Status: &status.Status{
			Code:    int32(code.Code_PERMISSION_DENIED),
			Message: "failed",
			Details: []*any.Any{},
		},
	}, nil
}

func main() {
	server := grpc.NewServer()
	authZ.RegisterAuthorizationServer(server, Auth{})

	//nolint: gosec // Test image, so it's fine to bind to everything.
	lis, err := net.Listen("tcp", ":6000")
	if err != nil {
		panic(err)
	}

	log.Printf("Running the External Authz service.")
	if err := server.Serve(lis); err != nil {
		panic(err)
	}
}
