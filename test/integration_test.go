package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"testing"
	"time"

	"gotest.tools/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"knative.dev/serving/pkg/apis/serving/v1alpha1"
	servingClientSet "knative.dev/serving/pkg/client/clientset/versioned/typed/serving/v1alpha1"
)

/*
These tests assume that there is a Kubernetes cluster running and that Knative
has been deployed. "utils/setup.sh" can be used to do that.
*/
const namespace string = "default"
const clusterURL string = "http://localhost:8080"
const domain string = "127.0.0.1.nip.io"

var kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")

func TestSimpleScenario(t *testing.T) {
	servingClient, err := KnativeServingClient(*kubeconfig)
	if err != nil {
		t.Fatal(err)
	}

	service, err := setupSimpleScenario(servingClient)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare the request
	client := &http.Client{}
	req, err := http.NewRequest("GET", clusterURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = fmt.Sprintf("%s.%s.%s", service.Name, namespace, domain)

	// Do the request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// The "hello world" service just returns "Hello World!"
	assert.Equal(t, string(respBody), "Hello World!\n")

	err = cleanSimpleScenario(servingClient, service.Name)
	if err != nil {
		t.Fatal(err)
	}
}

// Deploys a simple "Hello World" serving and returns it.
func setupSimpleScenario(servingClient *servingClientSet.ServingV1alpha1Client) (*v1alpha1.Service, error) {
	service := ExampleHelloWorldServing()

	// Make sure there's nothing left from previous tests
	err := cleanSimpleScenario(servingClient, service.Name)
	if err != nil {
		return nil, err
	}

	eventsServiceReady := make(chan struct{})
	stopChan := make(chan struct{})
	go watchForServiceReady(servingClient, service.Name, eventsServiceReady, stopChan)

	createdService, err := servingClient.Services(namespace).Create(&service)

	if err != nil {
		return nil, err
	}

	// Wait until the service is ready plus some time to make sure that Envoy
	// refreshed the config.
	<-eventsServiceReady
	time.Sleep(5 * time.Second)

	return createdService, nil
}

func watchForServiceReady(servingClient *servingClientSet.ServingV1alpha1Client,
	serviceName string,
	events chan<- struct{},
	stopChan <-chan struct{}) {

	restClient := servingClient.RESTClient()

	watchlist := cache.NewListWatchFromClient(restClient, "services", namespace, fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&v1alpha1.Service{},
		time.Second*1,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				service := obj.(*v1alpha1.Service)

				if service.Name == serviceName && service.Status.IsReady() {
					events <- struct{}{}
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				updatedService := newObj.(*v1alpha1.Service)

				if updatedService.Name == serviceName && updatedService.Status.IsReady() {
					events <- struct{}{}
				}
			},
		},
	)

	controller.Run(stopChan)
}

// Cleans the serving deployed in the simple scenario test.
// If the serving does not exist, it does not return an error, it simply does
// nothing.
func cleanSimpleScenario(servingClient *servingClientSet.ServingV1alpha1Client, serviceName string) error {
	err := servingClient.Services(namespace).Delete(serviceName, &v1.DeleteOptions{})

	if err != nil {
		// If the error is that the serving does not exist, just ignore.
		isNotFoundErr, err := regexp.MatchString(
			err.Error(),
			fmt.Sprintf("\"%s\" not found", serviceName),
		)

		if isNotFoundErr {
			return nil
		} else {
			return err
		}
	}

	return err
}
