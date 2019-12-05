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
	cache.set(&testCluster1, "some_ingress_name", "some_ingress_namespace")

	list := cache.list()

	assert.Equal(t, 1, len(list))
	assert.Equal(t, testCluster1.Name, list[0].(*envoy_api_v2.Cluster).Name)
}

func TestSetSeveralClusters(t *testing.T) {
	cache := newClustersCache()
	cache.set(&testCluster1, "some_ingress_name", "some_ingress_namespace")
	cache.set(&testCluster2, "some_ingress_name", "some_ingress_namespace")

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

func TestClustersExpire(t *testing.T) {
	cleanupInterval := time.Second
	cache := newClustersCacheWithExpAndCleanupIntervals(time.Second, cleanupInterval)
	cache.setExpiration(testCluster1.Name, "some_ingress_name", "some_ingress_namespace")
	time.Sleep(cleanupInterval + time.Second)

	list := cache.list()

	assert.Equal(t, 0, len(list))
}

func TestListWhenThereAreNoClusters(t *testing.T) {
	cache := newClustersCache()

	list := cache.list()

	assert.Equal(t, 0, len(list))
}
