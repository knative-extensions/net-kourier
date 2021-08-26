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

	envoy_api_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"k8s.io/apimachinery/pkg/util/wait"
)

var testCluster1 = envoy_api_v3.Cluster{
	Name: "test_cluster_1",
}

var testCluster2 = envoy_api_v3.Cluster{
	Name: "test_cluster_2",
}

func TestSetCluster(t *testing.T) {
	cache := newClustersCache()
	cache.set(&testCluster1, "some_ingress_name", "some_ingress_namespace")

	list := cache.list()

	assert.Assert(t, is.Len(list, 1))
	assert.Equal(t, testCluster1.Name, list[0].(*envoy_api_v3.Cluster).Name)
}

func TestSetSeveralClusters(t *testing.T) {
	cache := newClustersCache()
	cache.set(&testCluster1, "some_ingress_name", "some_ingress_namespace")
	cache.set(&testCluster2, "some_ingress_name", "some_ingress_namespace")

	list := cache.list()
	names := make([]string, 0, len(list))
	for _, cluster := range list {
		names = append(names, cluster.(*envoy_api_v3.Cluster).Name)
	}

	// Could be returned in any order
	expectedNames := []string{testCluster1.Name, testCluster2.Name}
	sort.Strings(expectedNames)
	sort.Strings(names)

	assert.DeepEqual(t, expectedNames, names)
}

func TestClustersExpire(t *testing.T) {
	interval := 10 * time.Millisecond
	cache := newClustersCacheWithExpAndCleanupIntervals(interval, interval)
	cache.set(&testCluster1, "some_ingress_name", "some_ingress_namespace")
	assert.Assert(t, is.Len(cache.list(), 1))

	// Wait for twice the interval and assert that the cluster is still there.
	time.Sleep(2 * interval)
	assert.Assert(t, is.Len(cache.list(), 1))

	// Mark the cluster to be expired.
	cache.setExpiration(testCluster1.Name, "some_ingress_name", "some_ingress_namespace")

	// The cluster should eventually disappear.
	wait.PollImmediate(interval, 5*time.Second, func() (bool, error) {
		return len(cache.list()) == 0, nil
	})
	assert.Assert(t, is.Len(cache.list(), 0))
}

func TestListWhenThereAreNoClusters(t *testing.T) {
	cache := newClustersCache()
	assert.Assert(t, is.Len(cache.list(), 0))
}
