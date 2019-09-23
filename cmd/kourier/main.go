package main

import (
	"courier/pkg/envoy"
	"courier/pkg/knative"
	"courier/pkg/kubernetes"
	log "github.com/sirupsen/logrus"
	"os"
)

const (
	nodeID         = "3scale-courier"
	gatewayPort    = 19001
	managementPort = 18000
)

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
	namespace := ""
	config := kubernetes.Config()
	kubernetesClient := kubernetes.NewKubernetesClient(config)
	knativeClient := knative.NewKnativeClient(config)

	eventsChan := make(chan string)

	stopChanEndpoints := make(chan struct{})
	go kubernetesClient.WatchChangesInEndpoints(namespace, eventsChan, stopChanEndpoints)

	stopChanClusterIngress := make(chan struct{})
	go knativeClient.WatchChangesInClusterIngress(namespace, eventsChan, stopChanClusterIngress)

	envoyXdsServer := envoy.NewEnvoyXdsServer(gatewayPort, managementPort, kubernetesClient)
	go envoyXdsServer.RunManagementServer()
	go envoyXdsServer.RunGateway()

	for {
		clusterIngresses, err := knativeClient.ClusterIngresses()
		if err != nil {
			panic(err)
		}

		envoyXdsServer.SetSnapshotForClusterIngresses(nodeID, clusterIngresses)

		<-eventsChan // Block until there's a change in the endpoints or servings
	}
}
