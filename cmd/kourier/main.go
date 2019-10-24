package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"kourier/pkg/envoy"
	"kourier/pkg/knative"
	"kourier/pkg/kubernetes"
	"os"
	"path/filepath"
)

const (
	nodeID         = "3scale-kourier-gateway"
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
	namespace := v1.NamespaceAll

	config := kubernetes.Config(*kubeconfig)
	kubernetesClient := kubernetes.NewKubernetesClient(config)
	knativeClient := knative.NewKnativeClient(config)

	eventsQueue := workqueue.New()

	stopChan := make(chan struct{})
	go kubernetesClient.WatchChangesInEndpoints(namespace, eventsQueue, stopChan)
	go knativeClient.WatchChangesInClusterIngress(namespace, eventsQueue, stopChan)
	go knativeClient.WatchChangesInIngress(namespace, eventsQueue, stopChan)

	envoyXdsServer := envoy.NewEnvoyXdsServer(gatewayPort, managementPort, kubernetesClient, knativeClient)
	go envoyXdsServer.RunManagementServer()
	go envoyXdsServer.RunGateway()

	// To make sure that the config is going to load at the start
	eventsQueue.Add(struct{}{})

	for {
		// Wait until there's an event to refresh the config.
		// For now, we don't act differently according to the type of event, we
		// just refresh the whole config.
		refreshConfigEvent, shutdown := eventsQueue.Get()
		eventsQueue.Done(refreshConfigEvent)

		if shutdown { // The queue was closed. Should never happen.
			panic("The events queue was closed")
		}

		ingressAccessors, err := knativeClient.IngressAccessors()
		if err != nil {
			log.Error(err)
		}

		envoyXdsServer.SetSnapshotForClusterIngresses(nodeID, ingressAccessors)
	}
}
