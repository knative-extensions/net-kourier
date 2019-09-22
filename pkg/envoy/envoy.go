package envoy

import (
	"context"
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
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	kubev1 "k8s.io/api/core/v1"
	v1alpha12 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	grpcMaxConcurrentStreams = 1000000
	namespaceHeader          = "Knative-Serving-Namespace"
	revisionHeader           = "Knative-Serving-Revision"
)

type EnvoyXdsServer struct {
	gatewayPort    uint
	managementPort uint
	kubeClient     kubernetes.KubernetesClient // TODO: let's try to remove this coupling later
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

func NewEnvoyXdsServer(gatewayPort uint, managementPort uint, kubeClient kubernetes.KubernetesClient) EnvoyXdsServer {
	ctx := context.Background()
	snapshotCache := cache.NewSnapshotCache(true, Hasher{}, nil)
	srv := xds.NewServer(snapshotCache, nil)

	return EnvoyXdsServer{
		gatewayPort:    gatewayPort,
		managementPort: managementPort,
		kubeClient:     kubeClient,
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

func (envoyXdsServer *EnvoyXdsServer) SetSnapshotForClusterIngresses(nodeId string, clusterIngresses *v1alpha12.ClusterIngressList) {
	var virtualHosts []*route.VirtualHost
	var routeCache []cache.Resource
	var clusterCache []cache.Resource

	for i, clusterIngress := range clusterIngresses.Items {

		routeName := getRouteName(clusterIngress)
		routeNamespace := getRouteNamespace(clusterIngress)
		log.WithFields(log.Fields{"name": routeName, "namespace": routeNamespace}).Info("Knative ClusterIngress found")

		for _, rule := range clusterIngress.Spec.Rules {

			var ruleRoute []*route.Route
			domains := rule.Hosts

			for _, httpPath := range rule.HTTP.Paths {

				path := "/"
				if httpPath.Path != "" {
					path = httpPath.Path
				}

				headers := httpPath.AppendHeaders

				var wrs []*route.WeightedCluster_ClusterWeight

				for _, split := range httpPath.Splits {

					endpointList, err := envoyXdsServer.kubeClient.EndpointsForRevision(split.ServiceNamespace, split.ServiceName)

					if err != nil {
						log.Errorf("%s", err)
						break
					}

					service, err := envoyXdsServer.kubeClient.ServiceForRevision(split.ServiceNamespace, split.ServiceName)

					if err != nil {
						log.Errorf("%s", err)
						break
					}

					var targetPort int32
					http2 := false
					for _, port := range service.Spec.Ports {
						if port.Port == split.ServicePort.IntVal || port.Name == split.ServicePort.StrVal {
							targetPort = port.TargetPort.IntVal
							http2 = port.Name == "http2" || port.Name == "h2c"
						}
					}

					lbEndpoints := lbEndpointsForKubeEndpoints(endpointList, targetPort)

					connectTimeout := 5 * time.Second
					cluster := clusterForRevision(split.ServiceName, connectTimeout, lbEndpoints, http2, path)
					clusterCache = append(clusterCache, &cluster)

					weightedCluster := weightedCluster(split.ServiceName, uint32(split.Percent), path, headers)

					wrs = append(wrs, &weightedCluster)

				}

				r := createRouteForRevision(routeName, i, path, wrs)

				ruleRoute = append(ruleRoute, &r)
				routeCache = append(routeCache, &r)

			}

			virtualHost := route.VirtualHost{
				Name:    routeName,
				Domains: domains,
				Routes:  ruleRoute,
			}

			virtualHosts = append(virtualHosts, &virtualHost)
		}

	}

	manager := httpConnectionManager(virtualHosts)
	l := envoyListener(&manager)
	listenerCache := []cache.Resource{&l}

	snapshotVersion, errUUID := uuid.NewUUID()
	if errUUID != nil {
		log.Error(errUUID)
		return
	}

	snapshot := cache.NewSnapshot(snapshotVersion.String(), nil, clusterCache, routeCache, listenerCache)

	err := envoyXdsServer.snapshotCache.SetSnapshot(nodeId, snapshot)
	if err != nil {
		log.Error(err)
	}
}

func createRouteForRevision(routeName string, i int, path string, wrs []*route.WeightedCluster_ClusterWeight) route.Route {
	r := route.Route{
		Name: routeName + "_" + strconv.Itoa(i),
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: path,
			},
		},
		Action: &route.Route_Route{Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: &route.WeightedCluster{
					Clusters: wrs,
				},
			},
		}},
	}
	return r
}

func getRouteNamespace(ingress v1alpha12.ClusterIngress) string {

	return ingress.Labels["serving.knative.dev/routeNamespace"]
}

func getRouteName(ingress v1alpha12.ClusterIngress) string {

	return ingress.Labels["serving.knative.dev/route"]
}

func lbEndpointsForKubeEndpoints(kubeEndpoints *kubev1.EndpointsList, targetPort int32) []*endpoint.LbEndpoint {
	var result []*endpoint.LbEndpoint

	for _, kubeEndpoint := range kubeEndpoints.Items {
		for _, subset := range kubeEndpoint.Subsets {

			for _, address := range subset.Addresses {

				serviceEndpoint := &core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.TCP,
							Address:  address.IP,
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: uint32(targetPort),
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

func clusterForRevision(revisionName string, connectTimeout time.Duration, lbEndpoints []*endpoint.LbEndpoint, http2 bool, path string) envoyv2.Cluster {

	cluster := envoyv2.Cluster{
		Name: revisionName + path,
		ClusterDiscoveryType: &envoyv2.Cluster_Type{
			Type: envoyv2.Cluster_STRICT_DNS,
		},
		ConnectTimeout: &connectTimeout,
		LoadAssignment: &envoyv2.ClusterLoadAssignment{
			ClusterName: revisionName + path,
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: lbEndpoints,
				},
			},
		},
	}

	if http2 {
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}

	return cluster
}

func weightedCluster(revisionName string, trafficPerc uint32, path string, headersToAdd map[string]string) route.WeightedCluster_ClusterWeight {

	var headers []*core.HeaderValueOption

	for k, v := range headersToAdd {

		header := core.HeaderValueOption{
			Header: &core.HeaderValue{
				Key:   k,
				Value: v,
			},
			Append: &types.BoolValue{
				Value: true,
			},
		}

		headers = append(headers, &header)

	}

	return route.WeightedCluster_ClusterWeight{
		Name: revisionName + path,
		Weight: &types.UInt32Value{
			Value: trafficPerc,
		},
		RequestHeadersToAdd: headers,
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
						PortValue: uint32(8080),
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
