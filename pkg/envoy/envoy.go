package envoy

import (
	"context"
	"fmt"
	"kourier/pkg/knative"
	"kourier/pkg/kubernetes"
	"net"
	"net/http"

	"knative.dev/pkg/network"

	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	v1alpha12 "knative.dev/serving/pkg/apis/networking/v1alpha1"
)

const (
	grpcMaxConcurrentStreams = 1000000
)

type EnvoyXdsServer struct {
	gatewayPort    uint
	managementPort uint
	kubeClient     kubernetes.KubernetesClient // TODO: let's try to remove this coupling later
	knativeClient  knative.KNativeClient
	ctx            context.Context
	server         xds.Server
	snapshotCache  cache.SnapshotCache
}

// Hasher returns node ID as an ID
type Hasher struct {
}

func (h Hasher) ID(node *core.Node) string {
	if node == nil {
		return "unknown"
	}
	return node.Id
}

func NewEnvoyXdsServer(gatewayPort uint, managementPort uint, kubeClient kubernetes.KubernetesClient, knativeClient knative.KNativeClient) EnvoyXdsServer {
	ctx := context.Background()
	snapshotCache := cache.NewSnapshotCache(true, Hasher{}, nil)
	srv := xds.NewServer(snapshotCache, nil)

	return EnvoyXdsServer{
		gatewayPort:    gatewayPort,
		managementPort: managementPort,
		kubeClient:     kubeClient,
		knativeClient:  knativeClient,
		ctx:            ctx,
		server:         srv,
		snapshotCache:  snapshotCache,
	}
}

// RunManagementServer starts an xDS server at the given Port.
func (envoyXdsServer *EnvoyXdsServer) RunManagementServer() {
	port := envoyXdsServer.managementPort
	server := envoyXdsServer.server

	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Error("Failed to listen")
	}

	// register services
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
	envoyv2.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	envoyv2.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	envoyv2.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	envoyv2.RegisterListenerDiscoveryServiceServer(grpcServer, server)

	log.Printf("Starting Management Server on Port %d\n", port)
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Errorf("%s", err)
		}
	}()
	<-envoyXdsServer.ctx.Done()
	grpcServer.GracefulStop()
}

// RunManagementGateway starts an HTTP gateway to an xDS server.
func (envoyXdsServer *EnvoyXdsServer) RunGateway() {
	port := envoyXdsServer.gatewayPort
	server := envoyXdsServer.server
	ctx := envoyXdsServer.ctx

	log.Printf("Starting HTTP/1.1 gateway on Port %d\n", port)
	httpServer := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: &xds.HTTPGateway{Server: server}}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	if err := httpServer.Shutdown(ctx); err != nil {
		panic(err)
	}
}

func (envoyXdsServer *EnvoyXdsServer) SetSnapshotForIngresses(nodeId string, Ingresses []v1alpha12.IngressAccessor) {
	snapshotVersion, errUUID := uuid.NewUUID()
	if errUUID != nil {
		log.Error(errUUID)
		return
	}

	localDomainName := network.GetClusterDomainName()

	caches := CachesForIngresses(Ingresses, &envoyXdsServer.kubeClient, localDomainName)

	snapshot := cache.NewSnapshot(
		snapshotVersion.String(),
		caches.endpoints,
		caches.clusters,
		caches.routes,
		caches.listeners,
	)

	err := envoyXdsServer.snapshotCache.SetSnapshot(nodeId, snapshot)

	if err != nil {
		log.Error(err)
	} else {
		for _, ingress := range Ingresses {
			err := envoyXdsServer.knativeClient.MarkIngressReady(ingress)
			if err != nil {
				log.Debug("Tried to mark an ingress as ready, but it no longer exists: ", err)
			}
		}
	}
}
