package main

import (
	"context"
	"flag"
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
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
	servingv1alpha1 "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"
	"net"
	"net/http"
	"os"
	"time"

	"path/filepath"
	k8s_cache "k8s.io/client-go/tools/cache"
)

var (
	config cache.SnapshotCache
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

// Pushes an event to the "events" channel received when an serving is added/deleted/updated.
func watchChangesInServings(servingClient *servingv1alpha1.ServingV1alpha1Client, namespace string, events chan<- string, stopChan <-chan struct{}) {
	restClient := servingClient.RESTClient()

	watchlist := k8s_cache.NewListWatchFromClient(restClient, "services", namespace,
		fields.Everything())

	_, controller := k8s_cache.NewInformer(
		watchlist,
		&v1alpha1.Service{},
		time.Second*1,
		k8s_cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				events <- "change"
			},

			DeleteFunc: func(obj interface{}) {
				events <- "change"
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				if oldObj != newObj {
					events <- "change"
				}
			},
		},
	)

	controller.Run(stopChan)
}

// Pushes an event to the "events" channel received when an endpoint is added/deleted/updated.
func watchChangesInEndpoints(kubernetesClient *kubernetes.Clientset, namespace string, events chan<- string, stopChan <-chan struct{}) {
	restClient := kubernetesClient.CoreV1().RESTClient()

	watchlist := k8s_cache.NewListWatchFromClient(restClient, "endpoints", namespace,
		fields.Everything())

	_, controller := k8s_cache.NewInformer(
		watchlist,
		&v12.Endpoints{},
		time.Second*1,
		k8s_cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				events <- "change"
			},

			DeleteFunc: func(obj interface{}) {
				events <- "change"
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				if oldObj != newObj {
					events <- "change"
				}
			},
		},
	)

	controller.Run(stopChan)
}

func main() {

	var kubeconfig *string
	version := 1
	ctx := context.Background()
	xdsConf := cache.NewSnapshotCache(true, Hasher{}, nil)

	srv := xds.NewServer(xdsConf, nil)

	go RunManagementServer(ctx, srv, 18000)
	go RunManagementGateway(ctx, srv, 19001)

	// Get the config, $HOME/.kube/config
	// TODO: Read from env var
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	var config *rest.Config
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		config, _ = rest.InClusterConfig()
	}

	servingClient, err := servingv1alpha1.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	eventsChan := make(chan string)

	stopChanEndpoints := make(chan struct{})
	go watchChangesInEndpoints(k8sClient, "serverless", eventsChan, stopChanEndpoints)

	stopChanServings := make(chan struct{})
	go watchChangesInServings(servingClient, "serverless", eventsChan, stopChanServings)

	for {
		serviceList, err := servingClient.Services("serverless").List(v1.ListOptions{})
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


			log.WithFields(log.Fields{"name": service.GetName(), "host": service.Status.URL.Host}, ).Info("Knative serving service found")

			r := route.Route{}
			wrs := []*route.WeightedCluster_ClusterWeight{}
			for _, traffic := range service.Status.Traffic {
				connectTimeout := 5 * time.Second

				listOptions := v1.ListOptions{LabelSelector: "serving.knative.dev/revision=" + traffic.RevisionName}
				endpointList, err := k8sClient.CoreV1().Endpoints(service.Namespace).List(listOptions)

				if err != nil {
					log.Errorf("%s", err)
					break
				}

				lbEndpoints := []*envoyEndpoint.LbEndpoint{}

				for _, endpoint := range endpointList.Items {
					for _, subset := range endpoint.Subsets {

						port, err := getHTTPPort(subset)

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
					ClusterSpecifier:            &route.RouteAction_WeightedClusters{
						WeightedClusters: &route.WeightedCluster{
							Clusters:             wrs,
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

		<- eventsChan // Block until there's a change in the endpoints or servings
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

func getHTTPPort(subset v12.EndpointSubset) (uint32, error) {

	for _, port := range subset.Ports {
		if port.Name == "http" {
			return uint32(port.Port), nil
		}
	}

	return 0, fmt.Errorf("http port not found")

}

func getRevision(service v1alpha1.Service) (string, error) {
	for _, traffic := range service.Status.Traffic {
		if traffic.Percent == 100 {
			return traffic.RevisionName, nil
		}
	}

	return "", fmt.Errorf("no revision with 100% of traffic")
}
