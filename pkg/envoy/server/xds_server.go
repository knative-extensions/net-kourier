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

package server

import (
	"context"
	"fmt"
	"net"

	cluster "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

const (
	grpcMaxConcurrentStreams = 1000000
)

type XdsServer struct {
	managementPort uint
	ctx            context.Context
	server         xds.Server
	snapshotCache  cache.SnapshotCache
}

func NewXdsServer(managementPort uint, callbacks xds.Callbacks) *XdsServer {
	ctx := context.Background()
	snapshotCache := cache.NewSnapshotCache(true, cache.IDHash{}, nil)
	srv := xds.NewServer(ctx, snapshotCache, callbacks)

	return &XdsServer{
		managementPort: managementPort,
		ctx:            ctx,
		server:         srv,
		snapshotCache:  snapshotCache,
	}
}

type healthServer struct {
	health.UnimplementedHealthServer
}

// Check implements the HealthServer interface.
func (healthServer) Check(context.Context, *health.HealthCheckRequest) (*health.HealthCheckResponse, error) {
	return &health.HealthCheckResponse{Status: health.HealthCheckResponse_SERVING}, nil
}

// RunManagementServer starts an xDS server at the given Port.
func (envoyXdsServer *XdsServer) RunManagementServer() error {
	port := envoyXdsServer.managementPort
	server := envoyXdsServer.server

	grpcServer := grpc.NewServer(
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{PermitWithoutStream: true}),
	)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// register services
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
	health.RegisterHealthServer(grpcServer, healthServer{})
	cluster.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	listener.RegisterListenerDiscoveryServiceServer(grpcServer, server)
	route.RegisterRouteDiscoveryServiceServer(grpcServer, server)

	errCh := make(chan error)
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-envoyXdsServer.ctx.Done():
		grpcServer.GracefulStop()
		return nil
	case err := <-errCh:
		return fmt.Errorf("failed to serve: %w", err)
	}
}

func (envoyXdsServer *XdsServer) SetSnapshot(nodeID string, snapshot cache.ResourceSnapshot) error {
	return envoyXdsServer.snapshotCache.SetSnapshot(context.Background(), nodeID, snapshot)
}
