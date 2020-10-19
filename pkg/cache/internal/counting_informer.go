package internal

import (
	"fmt"
	"time"

	"k8s.io/client-go/tools/cache"
)

// CountingInformer exposes a way to track the number of EventHandlers
// registered on an Informer.
type CountingInformer interface {
	cache.SharedIndexInformer
	CountEventHandlers() int
	RemoveEventHandler(id int) error
}

// HandlerCountingInformer implements the CountingInformer.
// It increments the count every time AddEventHandler is called,
// and decrements the count every time RemoveEventHandler is called.
//
// It doesn't actually RemoveEventHandlers because that feature is not implemented
// in client-go, but we're are naming it this way to suggest what the interface would look
// like if/when it does get added to client-go.
//
// We can get rid of this if apimachinery adds the ability to retrieve this from the SharedIndexInformer
// but until then, we have to track it ourselves
type HandlerCountingInformer struct {
	// Informer is the cached informer
	informer cache.SharedIndexInformer

	// count indicates the number of EventHandlers registered on the informer
	count int
}

func (i *HandlerCountingInformer) RemoveEventHandler(id int) error {
	i.count--
	fmt.Printf("decrement, count is %+v\n", i.count)
	return nil
}

func (i *HandlerCountingInformer) AddEventHandler(handler cache.ResourceEventHandler) {
	i.count++
	fmt.Printf("increment, count is %+v\n", i.count)
	i.informer.AddEventHandler(handler)
}

func (i *HandlerCountingInformer) CountEventHandlers() int {
	return i.count
}

func (i *HandlerCountingInformer) AddEventHandlerWithResyncPeriod(handler cache.ResourceEventHandler, resyncPeriod time.Duration) {
	i.count++
	i.informer.AddEventHandlerWithResyncPeriod(handler, resyncPeriod)
}
func (i *HandlerCountingInformer) AddIndexers(indexers cache.Indexers) error {
	return i.informer.AddIndexers(indexers)
}

func (i *HandlerCountingInformer) HasSynced() bool {
	return i.informer.HasSynced()
}

func (i *HandlerCountingInformer) GetStore() cache.Store {
	return i.informer.GetStore()
}

func (i *HandlerCountingInformer) GetController() cache.Controller {
	return i.informer.GetController()
}

func (i *HandlerCountingInformer) LastSyncResourceVersion() string {
	return i.informer.LastSyncResourceVersion()
}

func (i *HandlerCountingInformer) SetWatchErrorHandler(handler cache.WatchErrorHandler) error {
	return i.informer.SetWatchErrorHandler(handler)
}

func (i *HandlerCountingInformer) GetIndexer() cache.Indexer {
	return i.informer.GetIndexer()
}

func (i *HandlerCountingInformer) Run(stopCh <-chan struct{}) {
	i.informer.Run(stopCh)
}
