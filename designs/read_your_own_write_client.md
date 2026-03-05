Read your own write client
==========================

## Goals

* Any read from the client (`Get` or `List`) that starts after a write started will observe that write. The
  reason we consider the start and not the end time of the write to be the cutoff where reads must observe
  it is that we don't know when exactly the server applied it, as there is delay between the write being
  applied and the writer getting to know its write got applied.
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

* Any sort of consistency with writes originating from a different client - The only way to do this would be
  to do a get or list against the api at which point it doesn't make sense to have a cache-backed client at
  all
* Client reads observe writes that succeeded, but where the information if the write succeeded didn't make it
  back to the client, because for example the connection was interrupted. This is primarily for practical
  purposes, if the write doesn't get a response indicating success or failure back, it is impossible to tell if
  the write did or did not succeed
* Dealing with configurations where the cache doesn't contain all objects that are written, for example due to
  label or field selectors: This is essentially a misconfiguration and non-trivial to detect. We may or may not
  look into detecting this in the future

## Implementation

The implementation is gated behind a new and default-off client option `ConsistentReads bool`. If set, mutating
operations will:
* get sequentialized by gvk+key: This is purely to simplify implementation, we may or may not change this in the future
* block following get calls for the same gvk and object key and list calls for the gvk

Once the operation returned, it will:
1. Update the cache through a new internal-only `SetMinimumRVForGVKAndKey` method, which instructs it to block
  all subsequent Get calls for the given GVK and key and all list calls for the given gvk until the passed RV was observed
2. Unblock reads

A special challenge are Delete calls. There, the response is either the object including new RV if the object was
not deleted from storage or a metav1.Status without RV if the object was deleted from storage. To deal with this,
we will start deserializing the delete response into an unstructured and test which of the two it is. If it is the
object, we follow the same RV-waiting approach as for the other operations. If it is a success status, we will:
1. Update the cache through a new internal-only `AddRequiredDeleteForGVKKeyAndUID` method, which will make the cache
   append the UID to a per-gvk and key set. Whenever a delete event for a gvk and key is observed, the uid will be
   removed from the set. The cache then blocks gets on the gvk+key set being empty and lists on the gvk set being empty
2. Unblock reads

*Note:* The above only fulfills the goal `Reads do not get blocked on writes that started after they started` by assuming
that reads read the RV/deleted object set in the cache before any subsequent write calls `SetMinimumRVForGVKAndKey` or
`AddRequiredDeleteForGVKKeyAndUID`. The underlying information in the cache will be protected by a mutex and copied for
reads. While golang mutexes do not guarantee any acquisition order, writes entail a networking roundtrip, so we assume
that the read will always acquire the mutex before subsequent writes do. In the extremely unlikely case that they do not,
the result would only be a performance degradation (we wait for additional writes), not a correctness issue which is deemed
acceptable.
