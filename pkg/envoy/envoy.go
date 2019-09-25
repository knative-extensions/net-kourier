package envoy

import (
	"context"
	"fmt"
	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	accesslogv2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
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
	"kourier/pkg/kubernetes"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	grpcMaxConcurrentStreams = 1000000
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

func (envoyXdsServer *EnvoyXdsServer) SetSnapshotForClusterIngresses(nodeId string, Ingresses []v1alpha12.IngressAccessor) {
	var virtualHosts []*route.VirtualHost
	var routeCache []cache.Resource
	var clusterCache []cache.Resource

	for i, ingress := range Ingresses {

		routeName := getRouteName(ingress)
		routeNamespace := getRouteNamespace(ingress)

		log.WithFields(log.Fields{"name": routeName, "namespace": routeNamespace}).Info("Knative Ingress found")

		for _, rule := range ingress.GetSpec().Rules {

			var ruleRoute []*route.Route
			domains := rule.Hosts

			for _, httpPath := range rule.HTTP.Paths {

				path := "/"
				if httpPath.Path != "" {
					path = httpPath.Path
				}

				headersRev := httpPath.AppendHeaders

				var wrs []*route.WeightedCluster_ClusterWeight

				for _, split := range httpPath.Splits {

					headersSplit := split.AppendHeaders

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

					weightedCluster := weightedCluster(split.ServiceName, uint32(split.Percent), path, headersSplit)

					wrs = append(wrs, &weightedCluster)

				}

				attempts := 0
				var perTryTimeout time.Duration
				if httpPath.Retries != nil {
					attempts = httpPath.Retries.Attempts

					if httpPath.Retries.PerTryTimeout != nil {
						perTryTimeout = httpPath.Retries.PerTryTimeout.Duration
					}
				}

				r := createRouteForRevision(routeName, i, path, wrs, attempts, perTryTimeout, headersRev)

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

func createRouteForRevision(routeName string, i int, path string, wrs []*route.WeightedCluster_ClusterWeight, attempts int, perTryTimeout time.Duration, headersToAdd map[string]string) route.Route {

	// TODO: extract header generation
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
			RetryPolicy: createRetryPolicy(attempts, perTryTimeout),
		}},
		RequestHeadersToAdd:     headers,
	}

	return r
}

func createRetryPolicy(attempts int, perTryTimeout time.Duration) *route.RetryPolicy {
	if attempts > 0 {
		return &route.RetryPolicy{
			RetryOn: "5xx",
			NumRetries: &types.UInt32Value{
				Value: uint32(attempts),
			},
			PerTryTimeout: &perTryTimeout,
		}
	} else {
		return nil
	}
}

func getRouteNamespace(ingress v1alpha12.IngressAccessor) string {
	return ingress.GetLabels()["serving.knative.dev/routeNamespace"]
}

func getRouteName(ingress v1alpha12.IngressAccessor) string {
	return ingress.GetLabels()["serving.knative.dev/route"]
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

		AccessLog: accessLogs(),
	}
}

// Outputs to /dev/stdout using the default format
func accessLogs() []*accesslogv2.AccessLog {
	accessLogConfigFields := make(map[string]*types.Value)
	accessLogConfigFields["path"] = &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: "/dev/stdout",
		},
	}

	return []*accesslogv2.AccessLog{
		{
			Name: "envoy.file_access_log",
			ConfigType: &accesslogv2.AccessLog_Config{
				Config: &types.Struct{Fields: accessLogConfigFields},
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
