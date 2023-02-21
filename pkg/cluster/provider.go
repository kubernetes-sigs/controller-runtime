package cluster

import (
	"context"

	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/logical-cluster"
)

// Provider defines methods to retrieve, list, and watch fleet of clusters.
// The provider is responsible for discovering and managing the lifecycle of each
// cluster.
//
// Example: A Cluster API provider would be responsible for discovering and managing
// clusters that are backed by Cluster API resources, which can live
// in multiple namespaces in a single management cluster.
type Provider interface {
	Get(ctx context.Context, name logical.Name, opts ...Option) (Cluster, error)

	// List returns a list of logical clusters.
	// This method is used to discover the initial set of logical clusters
	// and to refresh the list of logical clusters periodically.
	List() ([]logical.Name, error)

	// Watch returns a Watcher that watches for changes to a list of logical clusters
	// and react to potential changes.
	Watch() (Watcher, error)
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
	//	 	The logical cluster was added or updated: a new RESTConfig is available, or needs to be refreshed.
	// - DELETED
	// 		The logical cluster was deleted: the cluster is removed.
	// - ERROR
	// 		An error occurred while watching the logical cluster: the cluster is removed.
	// - BOOKMARK
	// 		A periodic event is sent that contains no new data: ignored.
	Type watch.EventType

	// Name is the name of the logical cluster related to the event.
	Name logical.Name
}
