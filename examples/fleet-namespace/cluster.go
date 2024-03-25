package main

import (
	"context"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

type NamespacedCluster struct {
	clusterName string
	cluster.Cluster
}

func (c *NamespacedCluster) Name() string {
	return c.clusterName
}

func (c *NamespacedCluster) GetCache() cache.Cache {
	return &NamespacedCache{clusterName: c.clusterName, Cache: c.Cluster.GetCache()}
}

func (c *NamespacedCluster) GetClient() client.Client {
	return &NamespacedClient{clusterName: c.clusterName, Client: c.Cluster.GetClient()}
}

func (c *NamespacedCluster) GetEventRecorderFor(name string) record.EventRecorder {
	panic("implement me")
}

func (c *NamespacedCluster) GetAPIReader() client.Reader {
	return c.GetClient()
}

func (c *NamespacedCluster) Start(ctx context.Context) error {
	return nil // no-op as this is shared
}
