/*
Copyright 2018 The Kubernetes Authors.

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

package recorder

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-logr/logr"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime"
	eventsv1client "k8s.io/client-go/kubernetes/typed/events/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
)

// Provider is a recorder.Provider that records events to the k8s API server
// and to a logr Logger.
type Provider struct {
	lock    sync.RWMutex
	stopped bool

	// scheme to specify when creating a recorder
	scheme *runtime.Scheme
	// logger is the logger to use when logging diagnostic event info
	logger    logr.Logger
	evtClient eventsv1client.EventsV1Interface

	broadcasterOnce sync.Once
	broadcaster     events.EventBroadcaster
	stopBroadcaster bool
}

// NB(directxman12): this manually implements Stop instead of Being a runnable because we need to
// stop it *after* everything else shuts down, otherwise we'll cause panics as the leader election
// code finishes up and tries to continue emitting events.

// Stop attempts to stop this provider, stopping the underlying broadcaster
// if the broadcaster asked to be stopped.  It kinda tries to honor the given
// context, but the underlying broadcaster has an indefinite wait that doesn't
// return until all queued events are flushed, so this may end up just returning
// before the underlying wait has finished instead of cancelling the wait.
// This is Very Frustrating™.
func (p *Provider) Stop(shutdownCtx context.Context) {
	doneCh := make(chan struct{})

	go func() {
		// technically, this could start the broadcaster, but practically, it's
		// almost certainly already been started (e.g. by leader election).  We
		// need to invoke this to ensure that we don't inadvertently race with
		// an invocation of getBroadcaster.
		broadcaster := p.getBroadcaster()
		if p.stopBroadcaster {
			p.lock.Lock()
			broadcaster.Shutdown()
			p.stopped = true
			p.lock.Unlock()
		}
		close(doneCh)
	}()

	select {
	case <-shutdownCtx.Done():
	case <-doneCh:
	}
}

// getBroadcaster ensures that a broadcaster is started for this
// provider, and returns it.  It's threadsafe.
func (p *Provider) getBroadcaster() events.EventBroadcaster {
	// NB(directxman12): this can technically still leak if something calls
	// "getBroadcaster" (i.e. Emits an Event) but never calls Start, but if we
	// create the broadcaster in start, we could race with other things that
	// are started at the same time & want to emit events.  The alternative is
	// silently swallowing events and more locking, but that seems suboptimal.

	p.broadcasterOnce.Do(func() {
		if p.broadcaster == nil {
			p.broadcaster = events.NewBroadcaster(&events.EventSinkImpl{Interface: p.evtClient})
		}
		// TODO(clebs): figure out how to manage the context/channel that StartRecordingToSink needs inside the provider.
		p.broadcaster.StartRecordingToSink(nil)

		// TODO(clebs): figure out if we still need this and how the change would make sense.
		p.broadcaster.StartEventWatcher(
			func(obj runtime.Object) {
				if e, ok := obj.(*eventsv1.Event); ok {
					p.logger.V(1).Info(e.Note, "type", e.Type, "object", e.Regarding, "related", e.Related, "reason", e.Reason)
				} else {
					p.logger.V(1).Info("event watcher received an unsupported object type", "gvk", obj.GetObjectKind().GroupVersionKind().String())
				}
			})
	})

	return p.broadcaster
}

// NewProvider create a new Provider instance.
func NewProvider(config *rest.Config, httpClient *http.Client, scheme *runtime.Scheme, logger logr.Logger, broadcaster events.EventBroadcaster, stopWithProvider bool) (*Provider, error) {
	if httpClient == nil {
		panic("httpClient must not be nil")
	}

	eventsv1Client, err := eventsv1client.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to init client: %w", err)
	}

	p := &Provider{scheme: scheme, logger: logger, broadcaster: broadcaster, stopBroadcaster: stopWithProvider, evtClient: eventsv1Client}
	return p, nil
}

// GetEventRecorderFor returns an event recorder that broadcasts to this provider's
// broadcaster.  All events will be associated with a component of the given name.
func (p *Provider) GetEventRecorderFor(name string) record.EventRecorder {
	return &oldRecorder{
		newRecorder: &lazyRecorder{
			prov: p,
			name: name,
		},
	}
}

// GetEventRecorder returns an event recorder that broadcasts to this provider's
// broadcaster.  All events will be associated with a component of the given name.
func (p *Provider) GetEventRecorder(name string) events.EventRecorder {
	return &lazyRecorder{
		prov: p,
		name: name,
	}
}

// lazyRecorder is a recorder that doesn't actually instantiate any underlying
// recorder until the first event is emitted.
type lazyRecorder struct {
	prov *Provider
	name string

	recOnce sync.Once
	rec     events.EventRecorder
}

// ensureRecording ensures that a concrete recorder is populated for this recorder.
func (l *lazyRecorder) ensureRecording() {
	l.recOnce.Do(func() {
		broadcaster := l.prov.getBroadcaster()
		l.rec = broadcaster.NewRecorder(l.prov.scheme, l.name)
	})
}

func (l *lazyRecorder) Eventf(regarding runtime.Object, related runtime.Object, eventtype, reason, action, note string, args ...any) {
	l.ensureRecording()

	l.prov.lock.RLock()
	if !l.prov.stopped {
		l.rec.Eventf(regarding, related, eventtype, reason, action, note, args...)
	}
	l.prov.lock.RUnlock()
}

// oldRecorder is a wrapper around the events.EventRecorder that implements the old record.EventRecorder API.
// This is a temporary solution to support both the old and new events APIs without duplicating everything.
// Internally it calls the new events API from the old API funcs and no longer supported parameters are ignored (e.g. annotations).
type oldRecorder struct {
	newRecorder *lazyRecorder
}

func (l *oldRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	l.newRecorder.Eventf(object, nil, eventtype, reason, "unsupported", message)
}

func (l *oldRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...any) {
	l.newRecorder.Eventf(object, nil, eventtype, reason, "unsupported", messageFmt, args...)
}

func (l *oldRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...any) {
	l.newRecorder.Eventf(object, nil, eventtype, reason, "unsupported", messageFmt, args...)
}
