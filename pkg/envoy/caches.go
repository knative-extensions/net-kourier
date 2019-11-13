package envoy

import (
	"kourier/pkg/config"
	"kourier/pkg/knative"
	"net/http"
	"os"
	"strconv"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
	log "github.com/sirupsen/logrus"
	kubev1 "k8s.io/api/core/v1"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
)

type Caches struct {
	endpoints []cache.Resource
	clusters  []cache.Resource
	routes    []cache.Resource
	listeners []cache.Resource
}

// We need this because there might be Envoy clusters used by draining
// Listeners. The info of those clusters no longer appears in the Ingress
// object, so we need to store it.
// This is temporary. This should be extracted into its own module.
var clustersHistoric = newClustersCache()

type KubeClient interface {
	EndpointsForRevision(namespace string, serviceName string) (*kubev1.EndpointsList, error)
	ServiceForRevision(namespace string, serviceName string) (*kubev1.Service, error)
	GetSecret(namespace string, secretName string) (*kubev1.Secret, error)
}

func CachesForIngresses(Ingresses []v1alpha1.IngressAccessor, kubeClient KubeClient, localDomainName string, snapshotVersion string) Caches {
	var clusterLocalVirtualHosts []*route.VirtualHost
	var externalVirtualHosts []*route.VirtualHost

	var routeCache []cache.Resource

	for i, ingress := range Ingresses {
		routeName := getRouteName(ingress)
		routeNamespace := getRouteNamespace(ingress)

		log.WithFields(log.Fields{"name": routeName, "namespace": routeNamespace}).Info("Knative Ingress found")

		for _, rule := range ingress.GetSpec().Rules {

			var ruleRoute []*route.Route

			for _, httpPath := range rule.HTTP.Paths {

				path := "/"
				if httpPath.Path != "" {
					path = httpPath.Path
				}

				var wrs []*route.WeightedCluster_ClusterWeight

				for _, split := range httpPath.Splits {

					headersSplit := split.AppendHeaders

					endpointList, err := kubeClient.EndpointsForRevision(split.ServiceNamespace, split.ServiceName)

					if err != nil {
						log.Errorf("%s", err)
						break
					}
					service, err := kubeClient.ServiceForRevision(split.ServiceNamespace, split.ServiceName)

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

					privateLbEndpoints, publicLbEndpoints := lbEndpointsForKubeEndpoints(endpointList, targetPort)

					connectTimeout := 5 * time.Second
					cluster := clusterForRevision(split.ServiceName, connectTimeout, privateLbEndpoints, publicLbEndpoints, http2, path)

					clustersHistoric.set(split.ServiceName, path, split.ServiceNamespace, &cluster)

					weightedCluster := weightedCluster(split.ServiceName, uint32(split.Percent), path, headersSplit)

					wrs = append(wrs, &weightedCluster)
				}

				r := createRouteForRevision(routeName, i, &httpPath, wrs)

				ruleRoute = append(ruleRoute, &r)
				routeCache = append(routeCache, &r)

			}

			externalDomains := knative.ExternalDomains(&rule, localDomainName)
			virtualHost := route.VirtualHost{
				Name:    routeName,
				Domains: externalDomains,
				Routes:  ruleRoute,
			}

			// External should also be accessible internally
			internalDomains := append(knative.InternalDomains(&rule, localDomainName), externalDomains...)
			internalVirtualHost := route.VirtualHost{
				Name:    routeName,
				Domains: internalDomains,
				Routes:  ruleRoute,
			}

			if knative.RuleIsExternal(&rule, ingress.GetSpec().Visibility) {
				externalVirtualHosts = append(externalVirtualHosts, &virtualHost)
			}

			clusterLocalVirtualHosts = append(clusterLocalVirtualHosts, &internalVirtualHost)
		}
	}

	// Generate and append the internal kourier route for keeping track of the snapshot id deployed
	// to each envoy
	ikr := internalKourierRoute(snapshotVersion)
	ikvh := internalKourierVirtualHost(ikr)

	clusterLocalVirtualHosts = append(clusterLocalVirtualHosts, &ikvh)

	externalManager := newHttpConnectionManager(externalVirtualHosts)
	internalManager := newHttpConnectionManager(clusterLocalVirtualHosts)

	externalEnvoyListener, err := newExternalEnvoyListener(useHTTPSListener(), &externalManager, kubeClient)
	if err != nil {
		panic(err)
	}

	internalEnvoyListener, err := newInternalEnvoyListener(&internalManager)
	if err != nil {
		panic(err)
	}

	listenerCache := []cache.Resource{externalEnvoyListener, internalEnvoyListener}

	return Caches{
		endpoints: []cache.Resource{},
		clusters:  clustersHistoric.list(),
		routes:    routeCache,
		listeners: listenerCache,
	}
}

func internalKourierVirtualHost(ikr route.Route) route.VirtualHost {
	return route.VirtualHost{
		Name:    config.InternalKourierDomain,
		Domains: []string{config.InternalKourierDomain},
		Routes:  []*route.Route{&ikr},
	}
}

func internalKourierRoute(snapshotVersion string) route.Route {
	return route.Route{
		Name: config.InternalKourierDomain,
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Path{
				Path: config.InternalKourierPath,
			},
		},
		Action: &route.Route_DirectResponse{
			DirectResponse: &route.DirectResponseAction{Status: http.StatusOK},
		},
		ResponseHeadersToAdd: []*core.HeaderValueOption{
			{
				Header: &core.HeaderValue{
					Key:   config.InternalKourierHeader,
					Value: snapshotVersion,
				},
				Append: &wrappers.BoolValue{
					Value: true,
				},
			},
		},
	}
}

func getRouteNamespace(ingress v1alpha1.IngressAccessor) string {
	return ingress.GetLabels()["serving.knative.dev/routeNamespace"]
}

func getRouteName(ingress v1alpha1.IngressAccessor) string {
	return ingress.GetLabels()["serving.knative.dev/route"]
}

func lbEndpointsForKubeEndpoints(kubeEndpoints *kubev1.EndpointsList, targetPort int32) (privateLbEndpoints []*endpoint.LbEndpoint, publicLbEndpoints []*endpoint.LbEndpoint) {

	for _, kubeEndpoint := range kubeEndpoints.Items {

		for _, subset := range kubeEndpoint.Subsets {

			for _, address := range subset.Addresses {

				serviceEndpoint := &core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.SocketAddress_TCP,
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

				if kubeEndpoint.Labels["networking.internal.knative.dev/serviceType"] == "Private" {
					privateLbEndpoints = append(privateLbEndpoints, &lbEndpoint)
				} else if kubeEndpoint.Labels["networking.internal.knative.dev/serviceType"] == "Public" {
					publicLbEndpoints = append(publicLbEndpoints, &lbEndpoint)
				}
			}
		}
	}

	return privateLbEndpoints, publicLbEndpoints
}

func createRouteForRevision(routeName string, i int, httpPath *v1alpha1.HTTPIngressPath, wrs []*route.WeightedCluster_ClusterWeight) route.Route {
	path := "/"
	if httpPath.Path != "" {
		path = httpPath.Path
	}

	var routeTimeout time.Duration
	if httpPath.Timeout != nil {
		routeTimeout = httpPath.Timeout.Duration
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
			Timeout: ptypes.DurationProto(routeTimeout),
			UpgradeConfigs: []*route.RouteAction_UpgradeConfig{{
				UpgradeType: "websocket",
				Enabled:     &wrappers.BoolValue{Value: true},
			}},
			RetryPolicy: createRetryPolicyForRoute(httpPath),
		}},
		RequestHeadersToAdd: headersToAdd(httpPath.AppendHeaders),
	}

	return r
}

func weightedCluster(revisionName string, trafficPerc uint32, path string, headers map[string]string) route.WeightedCluster_ClusterWeight {
	return route.WeightedCluster_ClusterWeight{
		Name: revisionName + path,
		Weight: &wrappers.UInt32Value{
			Value: trafficPerc,
		},
		RequestHeadersToAdd: headersToAdd(headers),
	}
}

func headersToAdd(headers map[string]string) []*core.HeaderValueOption {
	var res []*core.HeaderValueOption

	for headerName, headerVal := range headers {
		header := core.HeaderValueOption{
			Header: &core.HeaderValue{
				Key:   headerName,
				Value: headerVal,
			},
			Append: &wrappers.BoolValue{
				Value: true,
			},
		}

		res = append(res, &header)

	}

	return res
}

func createRetryPolicyForRoute(httpPath *v1alpha1.HTTPIngressPath) *route.RetryPolicy {
	attempts := 0
	var perTryTimeout time.Duration
	if httpPath.Retries != nil {
		attempts = httpPath.Retries.Attempts

		if httpPath.Retries.PerTryTimeout != nil {
			perTryTimeout = httpPath.Retries.PerTryTimeout.Duration
		}
	}

	if attempts > 0 {
		return &route.RetryPolicy{
			RetryOn: "5xx",
			NumRetries: &wrappers.UInt32Value{
				Value: uint32(attempts),
			},
			PerTryTimeout: ptypes.DurationProto(perTryTimeout),
		}
	} else {
		return nil
	}
}

func clusterForRevision(revisionName string, connectTimeout time.Duration, privateLbEndpoints, publicLbEndpoints []*endpoint.LbEndpoint, http2 bool, path string) v2.Cluster {

	cluster := v2.Cluster{
		Name: revisionName + path,
		ClusterDiscoveryType: &v2.Cluster_Type{
			Type: v2.Cluster_STRICT_DNS,
		},
		ConnectTimeout: ptypes.DurationProto(connectTimeout),
		LoadAssignment: &v2.ClusterLoadAssignment{
			ClusterName: revisionName + path,
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: publicLbEndpoints,
					Priority:    1,
				},
				{
					LbEndpoints: privateLbEndpoints,
					Priority:    0,
				},
			},
		},
	}

	if http2 {
		cluster.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
	}

	return cluster
}

func useHTTPSListener() bool {
	return os.Getenv(envCertsSecretNamespace) != "" &&
		os.Getenv(envCertsSecretName) != ""
}
