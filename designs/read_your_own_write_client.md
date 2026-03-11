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
    * Waiting for the read to make it back into the cache may entail setting up an informer, which takes
      significant time

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
* Support `DeleteAllOf` - The apiservers response to DeleteAllOf is insufficient to implement this correctly. DeleteAllOf
  will error and instruct the user to use `List` and `Delete`
* Fail writes that use optimistic locking clientside - This could be done in the future, but is initially out of scope

## Implementation

The basic idea of the implementation is to make writes block concurrent reads to the same GVK+Key for Get and GVK for List
before the request is sent. If the request succeeds, the client provide the cache with either the returned resourceVersion
or the gvk+objectkey+uid of an object if it was deleted from storage, then unblock the reads. The cache will then copy this
RV/deleted object within the get/list and block the request until it observed it or the requests context times out.
It is important that we block before executing the write request and not after, because we can not know when exactly the server
commits it, only when it tells us having committed it.

The implementation is gated behind a `ReadYourOwnWrite *bool` `client.Options.Cache` setting. It will initially be disabled
by default, the goal is to enable it by default once we are confident in the implementation.

### Changes to the Cache

* Add an internal-only `SetMinimumRVForGVKAndKey(gvk schema.GroupVersionKind, key client.ObjectKey, rv int64)` method. Once
  called, all `Get` requests to the GVK+key and all `List` requests to the GVK will be blocked until the cache observed the
  passed rv or the request times out. The rv is copied before waiting to avoid subsequent calls to `SetMinimumRVForGVKAndKey`
  blocking the request
* Add an internal-only `AddRequiredDeleteForObject(obj client.Object) error` method. Once set, all `Get` requests to the GVK+key
  of object and all `List` requests to the GVK of object will be blocked until a delete event for GVK+UID of object was
  observed OR the requests context times our OR `RemoveRequiredDeleteForObject` is called for the same object. The object(s)
  whose deletion are awaited are copied before waiting to avoid subsequent `AddRequiredDeleteForObject` calls to block existing
  `Get`/`List` calls further. It will error if there is no existing started and synced informer for the passed object, as
  otherwise we can not observe the Delete event which will end up blocking all subsequent reads
* Add an internal-only `RemoveRequiredDeleteForObject(obj client.Object) error` which makes it not block reads for the passed
  objects GVK+UID anymore. This is required because it is possible that a delete event arrives in the cache before the Delete
  call finishes. This forces us to call `AddRequiredDeleteForObject` before executing the Delete call and hence we need to
  reverse that again if the call fails or the Object had a finalizer

### Changes to the Client

Add a new `readYourOwnWriteClient` wrapper to the client, which will wrap any new client that is constructed with
`options.Cache.ReadYourOwnWrite: new(true)`. This wrapper maintains a map of locks for gvk+key. It will wrap all
mutating operations and in their beginning acquire the lock for the requests object. If the request succeeds, it
will call the caches `SetMinimumRVForGVKAndKey` before returning. For `Delete`, it will additionally call
`AddRequiredDeleteForObject` before executing the call and if the call fails or the response contains an object, it
will then call `RemoveRequiredDeleteForObject`.
The wrapper also wraps all reading operations and block them until the current lock holder for the GVK+key in the
case of `Get` or all current lock holders for GVK in the case of `List` release their lock or the requests context
expires.

Add a new `DisableReadYourOwnWriteConsistency` option that can be used for either `Get` or `List` and disables the
above mentioned checks. This allows to disable the functionality for objects that are not cached.


## Open questions

How exactly do we implement internal-only methods? Options include:
1. Implement them on the type by do not add them to the published interface. This allows anyone to use these methods,
  but they need to dig for that and will probably aware that that usage is not supported
2. Move the implementations under `./internal` with the full interface visible there and put a small shim at
   the existing location that only provides the methods we want to be externally usable
