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

	authZ_v2 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	authZ_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

type AuthV2 struct{}

type AuthV3 struct{}

func (ea AuthV3) Check(ctx context.Context, ar *authZ_v3.CheckRequest) (*authZ_v3.CheckResponse, error) {
	if ar.Attributes.Request.Http.Path == "/success" || ar.Attributes.Request.Http.Path == "/healthz" {
		log.Print("TRUE")
		return &authZ_v3.CheckResponse{
			Status: &status.Status{
				Code: int32(code.Code_OK),
			},
		}, nil
	}

	log.Print("FAIL")
	return &authZ_v3.CheckResponse{
		Status: &status.Status{
			Code:    int32(code.Code_PERMISSION_DENIED),
			Message: "failed",
			Details: []*anypb.Any{},
		},
	}, nil
}

func (ea AuthV2) Check(ctx context.Context, ar *authZ_v2.CheckRequest) (*authZ_v2.CheckResponse, error) {
	if ar.Attributes.Request.Http.Path == "/success" || ar.Attributes.Request.Http.Path == "/healthz" {
		log.Print("TRUE")
		return &authZ_v2.CheckResponse{
			Status: &status.Status{
				Code: int32(code.Code_OK),
			},
		}, nil
	}

	log.Print("FAIL")
	return &authZ_v2.CheckResponse{
		Status: &status.Status{
			Code:    int32(code.Code_PERMISSION_DENIED),
			Message: "failed",
			Details: []*anypb.Any{},
		},
	}, nil
}

func main() {
	server := grpc.NewServer()
	authZ_v3.RegisterAuthorizationServer(server, AuthV3{})
	// For old envoy version such as proxyv2-ubi8:2.0.x, register auth server v2.
	authZ_v2.RegisterAuthorizationServer(server, AuthV2{})

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
