package main

// This is a really simple image for the kourier integration tests.

import (
	"context"
	"fmt"
	"log"
	"net"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"google.golang.org/genproto/googleapis/rpc/code"

	"github.com/golang/protobuf/ptypes/any"

	authZ "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
)

var logger log.Logger

const (
	grpcMaxConcurrentStreams = 1000000
	extAuthzPort             = 6000
)

type Auth struct {
	server grpc.Server
}

func (ea Auth) Check(ctx context.Context, ar *authZ.CheckRequest) (*authZ.CheckResponse, error) {

	if ar.Attributes.Request.Http.Path == "/success" {
		log.Print("TRUE")
		return &authZ.CheckResponse{
			Status: &status.Status{
				Code: int32(code.Code_OK),
			},
			HttpResponse: &authZ.CheckResponse_OkResponse{
				OkResponse: &authZ.OkHttpResponse{
					Headers: []*envoy_api_v2_core.HeaderValueOption{},
				},
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
		HttpResponse: &authZ.CheckResponse_DeniedResponse{},
	}, nil
}

func main() {

	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)
	auth := Auth{
		server: *grpcServer,
	}

	authZ.RegisterAuthorizationServer(grpcServer, auth)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", extAuthzPort))
	if err != nil {
		log.Println("Failed to listen")
	}

	log.Printf("Running the External Authz service.")
	if err = auth.server.Serve(lis); err != nil {
		log.Println(err)
	}
}
