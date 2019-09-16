package envoy

import (
	"context"
	"courier/pkg/knative"
	"courier/pkg/kubernetes"
	"fmt"
	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	kubev1 "k8s.io/api/core/v1"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
	"net"
	"net/http"
	"time"
)

const (
	grpcMaxConcurrentStreams = 1000000
	namespaceHeader          = "Knative-Serving-Namespace"
	revisionHeader           = "Knative-Serving-Revision"
)

type EnvoyXdsServer struct {
	gatewayPort            uint
	managementPort         uint
	kubeClient             kubernetes.KubernetesClient // TODO: let's try to remove this coupling later
	ctx                    context.Context
	server                 xds.Server
	currentSnapshotVersion int // TODO: overflow?
	snapshotCache          cache.SnapshotCache
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

func NewEnvoyXdsServer(gatewayPort uint, managementPort uint, kubeClient kubernetes.KubernetesClient) EnvoyXdsServer {
	ctx := context.Background()
	snapshotCache := cache.NewSnapshotCache(true, Hasher{}, nil)
	srv := xds.NewServer(snapshotCache, nil)

	return EnvoyXdsServer{
		gatewayPort:            gatewayPort,
		managementPort:         managementPort,
		kubeClient:             kubeClient,
		ctx:                    ctx,
		server:                 srv,
		currentSnapshotVersion: 1,
		snapshotCache:          snapshotCache,
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

func (envoyXdsServer *EnvoyXdsServer) SetSnapshotForKnativeServices(nodeId string, services *v1alpha1.ServiceList) {
	virtualHosts := []*route.VirtualHost{}
	var routeCache []cache.Resource
	var clusterCache []cache.Resource

	for _, service := range services.Items {

		if service.Status.URL == nil {
			log.Infof("Empty URL skipping")
			break
		}

		log.WithFields(log.Fields{"name": service.GetName(), "host": service.Status.URL.Host}).Info("Knative serving service found")

		wrs := []*route.WeightedCluster_ClusterWeight{}
		for _, traffic := range service.Status.Traffic {
			endpointList, err := envoyXdsServer.kubeClient.EndpointsForRevision(service.Namespace, traffic.RevisionName)

			if err != nil {
				log.Errorf("%s", err)
				break
			}

			lbEndpoints := lbEndpointsForKubeEndpoints(service, endpointList)

			connectTimeout := 5 * time.Second
			cluster := clusterForRevision(traffic.RevisionName, connectTimeout, lbEndpoints)

			if knative.ServiceHTTP2Enabled(service) {
				cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
			}
			clusterCache = append(clusterCache, &cluster)

			weightedCluster := weightedCluster(
				traffic.RevisionName, service.Namespace, uint32(traffic.Percent),
			)
			wrs = append(wrs, &weightedCluster)

		}

		r := routeWithWeightedClusters(service.Name, wrs)
		routeCache = append(routeCache, &r)

		domains, err := knative.DomainsFromService(&service)

		if err != nil {
			log.Errorf("cannot get domains for service %s : %s", service.Name, err)
		}

		virtualHost := route.VirtualHost{
			Name:    service.GetName(),
			Domains: domains,
			Routes:  []*route.Route{&r},
		}

		virtualHosts = append(virtualHosts, &virtualHost)

	}

	manager := httpConnectionManager(virtualHosts)

	l := envoyListener(&manager)

	listenerCache := []cache.Resource{&l}

	snapshot := cache.NewSnapshot(
		fmt.Sprintf("%d", envoyXdsServer.currentSnapshotVersion), nil, clusterCache, routeCache, listenerCache,
	)

	err := envoyXdsServer.snapshotCache.SetSnapshot(nodeId, snapshot)

	if err != nil {
		log.Error(err)
	} else {
		envoyXdsServer.currentSnapshotVersion++
	}
}

func lbEndpointsForKubeEndpoints(service v1alpha1.Service, kubeEndpoints *kubev1.EndpointsList) []*endpoint.LbEndpoint {
	result := []*endpoint.LbEndpoint{}

	for _, kubeEndpoint := range kubeEndpoints.Items {
		for _, subset := range kubeEndpoint.Subsets {

			port, err := knative.HTTPPortForEndpointSubset(service, subset)

			if err != nil {
				break
			}

			for _, address := range subset.Addresses {

				serviceEndpoint := &core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.TCP,
							Address:  address.IP,
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: port,
							},
							Ipv4Compat: true,
						},
					},
				}

				lbEndpoint := endpoint.LbEndpoint{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: serviceEndpoint,
						},
					},
				}
				result = append(result, &lbEndpoint)
			}
		}
	}

	return result
}

func clusterForRevision(revisionName string, connectTimeout time.Duration, lbEndpoints []*endpoint.LbEndpoint) envoyv2.Cluster {
	return envoyv2.Cluster{
		Name: "knative_" + revisionName,
		ClusterDiscoveryType: &envoyv2.Cluster_Type{
			Type: envoyv2.Cluster_STRICT_DNS,
		},
		ConnectTimeout: &connectTimeout,
		LoadAssignment: &envoyv2.ClusterLoadAssignment{
			ClusterName: "knative_" + revisionName,
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: lbEndpoints,
				},
			},
		},
	}
}

func weightedCluster(revisionName string, namespace string, trafficPerc uint32) route.WeightedCluster_ClusterWeight {
	return route.WeightedCluster_ClusterWeight{
		Name: "knative_" + revisionName,
		Weight: &types.UInt32Value{
			Value: trafficPerc,
		},
		RequestHeadersToAdd: []*core.HeaderValueOption{{
			Header: &core.HeaderValue{
				Key:   namespaceHeader,
				Value: namespace,
			},
			Append: &types.BoolValue{
				Value: true,
			},
		},
			{
				Header: &core.HeaderValue{
					Key:   revisionHeader,
					Value: revisionName,
				},
				Append: &types.BoolValue{
					Value: true,
				},
			},
		},
	}
}

func routeWithWeightedClusters(serviceName string, weightedClusters []*route.WeightedCluster_ClusterWeight) route.Route {
	return route.Route{
		Name: "knative_" + serviceName,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: "/",
			},
		},
		Action: &route.Route_Route{Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: &route.WeightedCluster{
					Clusters: weightedClusters,
				},
			},
		}},
	}
}

func httpConnectionManager(virtualHosts []*route.VirtualHost) v2.HttpConnectionManager {
	return v2.HttpConnectionManager{
		CodecType:  v2.AUTO,
		StatPrefix: "ingress_http",
		RouteSpecifier: &v2.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoyv2.RouteConfiguration{
				Name:         "local_route",
				VirtualHosts: virtualHosts,
			},
		},
		HttpFilters: []*v2.HttpFilter{
			{
				Name: util.Router,
			},
		},
	}
}

func envoyListener(httpConnectionManager *v2.HttpConnectionManager) envoyv2.Listener {
	pbst, err := util.MessageToStruct(httpConnectionManager)
	if err != nil {
		panic(err)
	}

	return envoyv2.Listener{
		Name: "listener_0",
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: uint32(80),
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name:       util.HTTPConnectionManager,
				ConfigType: &listener.Filter_Config{Config: pbst},
			}},
		}},
	}
}
