package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"knative.dev/serving/pkg/apis/networking/v1alpha1"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"
	"kourier/pkg/kubernetes"
	"os"
	"path/filepath"
)

const (
	nodeID         = "3scale-kourier"
	gatewayPort    = 19001
	managementPort = 18000
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

var kubeconfig *string

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)

	// Parse flags
	if home := homeDir(); home != "" {
		kubeconfig = flag.String(
			"kubeconfig",
			filepath.Join(home, ".kube", "config"),
			"(optional) absolute path to the kubeconfig file",
		)
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
}

func main() {
	namespace := ""
	config := kubernetes.Config(*kubeconfig)
	kubernetesClient := kubernetes.NewKubernetesClient(config)
	knativeClient := knative.NewKnativeClient(config)

	eventsChan := make(chan struct{})

	stopChan := make(chan struct{})
	go kubernetesClient.WatchChangesInEndpoints(namespace, eventsChan, stopChan)
	go knativeClient.WatchChangesInClusterIngress(namespace, eventsChan, stopChan)
	go knativeClient.WatchChangesInIngress(namespace, eventsChan, stopChan)

	envoyXdsServer := envoy.NewEnvoyXdsServer(gatewayPort, managementPort, kubernetesClient, knativeClient)
	go envoyXdsServer.RunManagementServer()
	go envoyXdsServer.RunGateway()

	for {

		ingresses, err := knativeClient.Ingresses()
		if err != nil {
			log.Error(err)
		}

		clusterIngresses, err := knativeClient.ClusterIngresses()
		if err != nil {
			log.Error(err)
		}

		var ingressList []v1alpha1.IngressAccessor

		for i, _ := range ingresses {
			ingressList = append(ingressList, v1alpha1.IngressAccessor(&ingresses[i]))
		}

		for i, _ := range clusterIngresses {
			ingressList = append(ingressList, v1alpha1.IngressAccessor(&clusterIngresses[i]))
		}

		envoyXdsServer.SetSnapshotForClusterIngresses(nodeID, ingressList)

		<-eventsChan // Block until there's a change in the endpoints or servings
	}
}
