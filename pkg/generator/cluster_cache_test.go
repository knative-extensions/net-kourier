/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generator

import (
	"sort"
	"testing"
	"time"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"go.uber.org/zap"
	"gotest.tools/assert"
)

var testCluster1 = envoy_api_v2.Cluster{
	Name: "test_cluster_1",
}

var testCluster2 = envoy_api_v2.Cluster{
	Name: "test_cluster_2",
}

func TestSetCluster(t *testing.T) {
	logger := zap.S()

	cache := newClustersCache(logger)
	cache.set(&testCluster1, "some_ingress_name", "some_ingress_namespace")

	list := cache.list()

	assert.Equal(t, 1, len(list))
	assert.Equal(t, testCluster1.Name, list[0].(*envoy_api_v2.Cluster).Name)
}

func TestSetSeveralClusters(t *testing.T) {
	logger := zap.S()

	cache := newClustersCache(logger)
	cache.set(&testCluster1, "some_ingress_name", "some_ingress_namespace")
	cache.set(&testCluster2, "some_ingress_name", "some_ingress_namespace")

	list := cache.list()
	names := make([]string, 0, len(list))
	for _, cluster := range list {
		names = append(names, cluster.(*envoy_api_v2.Cluster).Name)
	}

	assert.Equal(t, 2, len(list))

	// Could be returned in any order
	expectedNames := []string{testCluster1.Name, testCluster2.Name}
	sort.Strings(expectedNames)
	sort.Strings(names)
	assert.DeepEqual(t, expectedNames, names)
}

func TestClustersExpire(t *testing.T) {
	logger := zap.S()

	cleanupInterval := time.Second
	cache := newClustersCacheWithExpAndCleanupIntervals(time.Second, cleanupInterval, logger)
	cache.setExpiration(testCluster1.Name, "some_ingress_name", "some_ingress_namespace")
	time.Sleep(cleanupInterval + time.Second)

	list := cache.list()

	assert.Equal(t, 0, len(list))
}

func TestListWhenThereAreNoClusters(t *testing.T) {
	logger := zap.S()

	cache := newClustersCache(logger)

	list := cache.list()

	assert.Equal(t, 0, len(list))
}
