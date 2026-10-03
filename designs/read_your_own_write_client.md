Read your own write client
==========================

## Background

Controller-Runtimes default client writes to the api and reads from an informercache. As a result, performing a read
right after a write tends to not return that write since it hasn't made it back to the informer cache yet. This
leads to bugs and workarounds like replacing the read portion of the client to read from the API instead. The goal of
this proposal is to provide the same consistency in the cache-reading client a live client would with regards to
writes performed through the same client.

## Goals

* Any read from the client (`Get` or `List`) contains all writes performed through the same client that were
  committed by the server at the time the read happened
* Reads do not get blocked on writes that started after they started, as otherwise they may end up getting
  blocked indefinitely if many writes are happening.
* Writes themselves do not get blocked waiting for them to be observable by reads because:
    * This may lead to successful writes returning an error. While that should not result in incorrect behavior
      of a controller as controllers are expected to be idempotent, it would lead to performance degradation, as
      the error will typical lead to a retry with backoff and any work done before encountering it has to be
      re-done

## Non-Goals

* Any sort of consistency with writes originating from a client in another binary - The only way to do this would be
  to do a get or list against the api at which point it doesn't make sense to have a cache-backed client at
  all
* Consistency between multiple clients constructed using the same informercache
* Consistency for writes that succeeded, but where the information if the write succeeded didn't make it
  back to the client for any reason like the connection breaking. This is primarily for practical
  purposes, if the write doesn't get a response indicating success or failure back, it is impossible to tell if
  the write did or did not succeed
* Automatically dealing with configurations where the cache doesn't contain all objects that are written, for
  example because its label or field selector doesn't match them. We will provide an option for this case and
  may or may not try to detect this correctly in the future
* Dealing with configurations where a Transform strips off required fields like RV or UID
* Support `DeleteAllOf` - The apiservers response to DeleteAllOf is insufficient to implement this correctly. DeleteAllOf
  will error and instruct the user to use `List` and `Delete`
* Fail writes that use optimistic locking clientside - This could be done in the future, but is initially out of scope

### Future goals

* Allow more fine-granular configuration about what to wait on as this warrants its own discussion
* Optimize blocking of namespaced list to only consider objects in the namespace

## Implementation

The basic idea of the implementation is to make writes block concurrent reads to the same GVK+Key for Get and GVK for List
before the request is sent. If the request succeeds, the client will store  either the returned resourceVersion
or the gvk+objectkey+uid of an object if it was deleted from storage. The client will then copy this RV/deleted object within
the get/list and block the request until it observed it or the requests context times out.
It is important that we block before executing the write request and not after, because we can not know when exactly the server
commits it, only when it tells us that it commited it.

The implementation is gated behind a `EnableReadYourWritesConsistency: *bool` `client.Options.Cache` setting. It will initially
be disabled by default, the goal is to enable it by default once we are confident in the implementation.


### Changes to the Client

Add a new `consistentClient` wrapper to the client, which will wrap any new client that is constructed with
`options.Cache.EnableReadYourWritesConsistency: new(true)`. This wrapper is responsible for ensuring that a) reads
wait for all writes that started before them to the resource(s) they are reading to finish and b) reads then wait
for the cache to catch up to a state after the write. a) is achieved using the following:

```
type WriteBarriers interface {
	// Begin keeps the passed key locked until the returned release func was called.
	Begin(key types.NamespacedName) (release func())

	// Seal seals the currently-active set of locks to key and returns a channel that
	// closes when they are done.
	Seal(key types.NamespacedName) <-chan struct{}

	// SealAll is Seal for all keys.
	SealAll() []<-chan struct{}
}
```
The reason we need this and can not use a simple mutex is that a mutex would allow us to wait for at most one write and
it would serialize all writes.

b) is achieved by adding a `consistencyHandler` that is added as an eventhandler to the cache and exposes the
following methods:
```
func (h *ConsistencyHandler) AddPendingDelete(key types.NamespacedName, uid types.UID)
func (h *ConsistencyHandler) OnAdd(raw any, _ bool)
func (h *ConsistencyHandler) OnDelete(raw any)
func (h *ConsistencyHandler) OnUpdate(_, newObj any)
func (h *ConsistencyHandler) Register(ctx context.Context, informer cacheapi.Informer) error
func (h *ConsistencyHandler) Registered() bool
func (h *ConsistencyHandler) RemovePendingDelete(key types.NamespacedName, uid types.UID)
func (h *ConsistencyHandler) SetMinimumRV(key types.NamespacedName, rv int64)
func (h *ConsistencyHandler) WaitForGet(ctx context.Context, key types.NamespacedName) error
func (h *ConsistencyHandler) WaitForList(ctx context.Context) error
```

`Delete` requests will then call `AddPendingDelete` before issuing the request and `RemovePendingDelete` if the request
didn't succeed or the object didn't get removed from storage and a RV is returned. The ordering is important here,
if we added the Delete after the request finished, it is theoretically possible that the handler already observed it
and will then prevent all reads waiting for an event it will never receive. All other write requests call `SetMinimumRV`.

`Get`/`List` call `WaitForGet`/`WaitForList` which will copy the current set of deletes that is being waited on as well
as the `ResoureVersion` from `SetMinimumRV` and wait for the cache to observe them. If the cache already observed them,
it will return right away.

Add a new `DisableReadYourWritesConsistency` option that can be used for any client method and disables the
above-described logic for that call.

### Changes to the cache

Create a new package `pkg/cache/cacheapi` that contains the interface definitions that are currently in
`pkg/cache` as well as the interface definitions from `pkg/client` they reference. This allows the client
package to use the `Informer` interface without a cyclic import. The places where these interfaces are
currently defined will add an alias to the new location to avoid a breaking change.
