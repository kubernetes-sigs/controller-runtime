package watch

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func NewDynamicWatch(config rest.Config) (*dynamicWatch, chan event.GenericEvent, error) {
	dw := &dynamicWatch{events: make(chan event.GenericEvent)}

	restMapper, err := apiutil.NewDiscoveryRESTMapper(&config)
	if err != nil {
		return nil, nil, err
	}

	client, err := dynamic.NewForConfig(&config)
	if err != nil {
		return nil, nil, err
	}

	dw.restMapper = restMapper
	dw.config = config
	dw.client = client
	return dw, dw.events, nil
}

type dynamicWatch struct {
	config     rest.Config
	client     dynamic.Interface
	restMapper meta.RESTMapper
	events     chan event.GenericEvent
}

func (dw *dynamicWatch) newDynamicClient(gvk schema.GroupVersionKind) (dynamic.ResourceInterface, error) {
	mapping, err := dw.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	return dw.client.Resource(mapping.Resource), nil
}

// Add registers a watch for changes to 'trigger' filtered by 'options' to raise an event on 'target'
func (dw *dynamicWatch) Add(trigger schema.GroupVersionKind, options metav1.ListOptions, target metav1.ObjectMeta) error {
	log := log.Log

	client, err := dw.newDynamicClient(trigger)
	if err != nil {
		return fmt.Errorf("creating client for (%s): %v", trigger.String(), err)
	}

	events, err := client.Watch(options)

	if err != nil {
		log.WithValues("kind", trigger.String()).WithValues("namespace", target.Namespace).WithValues("labels", options.LabelSelector).Error(err, "adding watch to dynamic client")
		return fmt.Errorf("adding watch to dynamic client: %v", err)
	}

	log.WithValues("kind", trigger.String()).WithValues("namespace", target.Namespace).WithValues("options", options.String()).V(2).Info("watch registered")
	eventChan := events.ResultChan()

	go func() {
		for clientEvent := range eventChan {
			if clientEvent.Object == nil || clientEvent.Type == watch.Added {
				continue
			}
			log.WithValues("type", clientEvent.Type).WithValues("kind", trigger.String()).Info("broadcasting event")
			dw.events <- event.GenericEvent{Object: clientEvent.Object, Meta: &target}
		}
	}()

	return nil
}
