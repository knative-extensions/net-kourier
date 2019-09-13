package main

import (
	"context"
	"fmt"
	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyEndpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	grpcMaxConcurrentStreams = 1000000
	nodeID                   = "3scale-courier"
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
}

func main() {
	version := 1
	ctx := context.Background()
	xdsConf := cache.NewSnapshotCache(true, Hasher{}, nil)

	srv := xds.NewServer(xdsConf, nil)

	go RunManagementServer(ctx, srv, 18000)
	go RunManagementGateway(ctx, srv, 19001)

	namespace := "serverless"

	config := Config()
	kubernetesClient := NewKubernetesClient(config)
	knativeClient := NewKnativeClient(config)

	eventsChan := make(chan string)

	stopChanEndpoints := make(chan struct{})
	go kubernetesClient.WatchChangesInEndpoints(namespace, eventsChan, stopChanEndpoints)

	stopChanServings := make(chan struct{})
	go knativeClient.WatchChangesInServices(namespace, eventsChan, stopChanServings)

	for {
		serviceList, err := knativeClient.Services(namespace)
		if err != nil {
			panic(err)
		}

		virtualHosts := []*route.VirtualHost{}
		var routeCache []cache.Resource
		var clusterCache []cache.Resource

		for _, service := range serviceList.Items {

			if service.Status.URL == nil {
				log.Infof("Empty URL skipping")
				break
			}

			log.WithFields(log.Fields{"name": service.GetName(), "host": service.Status.URL.Host}).Info("Knative serving service found")

			r := route.Route{}
			wrs := []*route.WeightedCluster_ClusterWeight{}
			for _, traffic := range service.Status.Traffic {
				connectTimeout := 5 * time.Second

				endpointList, err := kubernetesClient.EndpointsForRevision(service.Namespace, traffic.RevisionName)

				if err != nil {
					log.Errorf("%s", err)
					break
				}

				lbEndpoints := []*envoyEndpoint.LbEndpoint{}

				for _, endpoint := range endpointList.Items {
					for _, subset := range endpoint.Subsets {

						port, err := HTTPPortForEndpointSubset(subset)

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

							lbEndpoint := envoyEndpoint.LbEndpoint{
								HostIdentifier: &envoyEndpoint.LbEndpoint_Endpoint{
									Endpoint: &envoyEndpoint.Endpoint{
										Address: serviceEndpoint,
									},
								},
							}
							lbEndpoints = append(lbEndpoints, &lbEndpoint)

						}

					}
				}

				cluster := envoyv2.Cluster{
					Name: "knative_" + traffic.RevisionName,
					//AltStatName: "",
					ClusterDiscoveryType: &envoyv2.Cluster_Type{
						Type: envoyv2.Cluster_STRICT_DNS,
					},
					ConnectTimeout: &connectTimeout,
					LoadAssignment: &envoyv2.ClusterLoadAssignment{
						ClusterName: "knative_" + traffic.RevisionName,
						Endpoints: []*envoyEndpoint.LocalityLbEndpoints{
							{
								LbEndpoints: lbEndpoints,
							},
						},
					},
				}

				clusterCache = append(clusterCache, &cluster)

				wr := route.WeightedCluster_ClusterWeight{
					Name: "knative_" + traffic.RevisionName,
					Weight: &types.UInt32Value{
						Value: uint32(traffic.Percent),
					},
					RequestHeadersToAdd: []*core.HeaderValueOption{{
						Header: &core.HeaderValue{
							Key:   "Knative-Serving-Namespace",
							Value: service.Namespace,
						},
						Append: &types.BoolValue{
							Value: true,
						},
					},
						{
							Header: &core.HeaderValue{
								Key:   "Knative-Serving-Revision",
								Value: traffic.RevisionName,
							},
							Append: &types.BoolValue{
								Value: true,
							},
						},
					},
				}

				wrs = append(wrs, &wr)

			}

			r = route.Route{
				Name: "knative_" + service.Name,
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: "/",
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

			routeCache = append(routeCache, &r)

			virtualHost := route.VirtualHost{
				Name:    service.GetName(),
				Domains: []string{service.Status.URL.Host},
				Routes:  []*route.Route{&r},
			}

			virtualHosts = append(virtualHosts, &virtualHost)

		}

		manager := &hcm.HttpConnectionManager{
			CodecType:  hcm.AUTO,
			StatPrefix: "ingress_http",
			RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
				RouteConfig: &envoyv2.RouteConfiguration{
					Name:         "local_route",
					VirtualHosts: virtualHosts,
				},
			},
			HttpFilters: []*hcm.HttpFilter{
				{
					Name: util.Router,
				},
			},
		}

		//TODO: ?????
		pbst, err := util.MessageToStruct(manager)
		if err != nil {
			panic(err)
		}

		listener := &envoyv2.Listener{
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
		version = version + 1

		listenerCache := []cache.Resource{listener}

		snapshot := cache.NewSnapshot(fmt.Sprintf("%d", version), nil, clusterCache, routeCache, listenerCache)

		err = xdsConf.SetSnapshot(nodeID, snapshot)
		if err != nil {
			log.Error(err)
		}

		<-eventsChan // Block until there's a change in the endpoints or servings
	}
}

// RunManagementServer starts an xDS server at the given Port.
func RunManagementServer(ctx context.Context, server xds.Server, port uint) {
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
			fmt.Errorf("%s", err)
		}
	}()
	<-ctx.Done()
	grpcServer.GracefulStop()
}

// RunManagementGateway starts an HTTP gateway to an xDS server.
func RunManagementGateway(ctx context.Context, srv xds.Server, port uint) {
	log.Printf("Starting HTTP/1.1 gateway on Port %d\n", port)
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: &xds.HTTPGateway{Server: srv}}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	if err := server.Shutdown(ctx); err != nil {
		panic(err)
	}
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
