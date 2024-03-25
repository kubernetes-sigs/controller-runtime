package cluster

import (
	"context"

	"k8s.io/apimachinery/pkg/watch"
)

// Provider defines methods to retrieve, list, and watch fleet of clusters.
// The provider is responsible for discovering and managing the lifecycle of each
// cluster.
//
// Example: A Cluster API provider would be responsible for discovering and managing
// clusters that are backed by Cluster API resources, which can live
// in multiple namespaces in a single management cluster.
type Provider interface {
	// Get returns a cluster for the given identifying cluster name. The
	// options are passed to the cluster constructor in case the cluster has
	// not been created yet. Get returns an existing cluster if it has been
	// created before.
	Get(ctx context.Context, clusterName string, opts ...Option) (Cluster, error)

	// List returns a list of known identifying clusters names.
	// This method is used to discover the initial set of known cluster names
	// and to refresh the list of cluster names periodically.
	List(ctx context.Context) ([]string, error)

	// Watch returns a Watcher that watches for changes to a list of known clusters
	// and react to potential changes.
	Watch(ctx context.Context) (Watcher, error)
}

// Watcher watches for changes to clusters and provides events to a channel
// for the Manager to react to.
type Watcher interface {
	// Stop stops watching. Will close the channel returned by ResultChan(). Releases
	// any resources used by the watch.
	Stop()

	// ResultChan returns a chan which will receive all the events. If an error occurs
	// or Stop() is called, the implementation will close this channel and
	// release any resources used by the watch.
	ResultChan() <-chan WatchEvent
}

// WatchEvent is an event that is sent when a cluster is added, modified, or deleted.
type WatchEvent struct {
	// Type is the type of event that occurred.
	//
	// - ADDED or MODIFIED
	//	 	The cluster was added or updated: a new RESTConfig is available, or needs to be refreshed.
	// - DELETED
	// 		The cluster was deleted: the cluster is removed.
	// - ERROR
	// 		An error occurred while watching the cluster: the cluster is removed.
	// - BOOKMARK
	// 		A periodic event is sent that contains no new data: ignored.
	Type watch.EventType

	// ClusterName is the identifying name of the cluster related to the event.
	ClusterName string
}
