package generator

import (
	"sort"
	"testing"
	"time"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"gotest.tools/assert"
)

var testCluster1 = envoy_api_v2.Cluster{
	Name: "test_cluster_1",
}

var testCluster2 = envoy_api_v2.Cluster{
	Name: "test_cluster_2",
}

func TestSetCluster(t *testing.T) {
	cache := newClustersCache()
	cache.set("helloworld", "/", "test_namespace", &testCluster1)

	list := cache.list()

	assert.Equal(t, 1, len(list))
	assert.Equal(t, testCluster1.Name, list[0].(*envoy_api_v2.Cluster).Name)
}

func TestSetSeveralClusters(t *testing.T) {
	cache := newClustersCache()
	cache.set("helloworld_1", "/", "test_namespace", &testCluster1)
	cache.set("helloworld_2", "/", "test_namespace", &testCluster2)

	list := cache.list()
	var names []string
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

func TestSetExistingClusterReplacesIt(t *testing.T) {
	cache := newClustersCache()
	cache.set("helloworld", "/", "test_namespace", &testCluster1)
	cache.set("helloworld", "/", "test_namespace", &testCluster2) // should replace

	list := cache.list()

	assert.Equal(t, 1, len(list))
	assert.Equal(t, testCluster2.Name, list[0].(*envoy_api_v2.Cluster).Name)
}

func TestClustersExpire(t *testing.T) {
	cleanupInterval := time.Second
	cache := newClustersCacheWithExpAndCleanupIntervals(time.Second, cleanupInterval)
	cache.set("helloworld", "/", "test_namespace", &testCluster1)
	time.Sleep(cleanupInterval + time.Second)

	list := cache.list()

	assert.Equal(t, 0, len(list))
}

func TestListWhenThereAreNoClusters(t *testing.T) {
	cache := newClustersCache()

	list := cache.list()

	assert.Equal(t, 0, len(list))
}
