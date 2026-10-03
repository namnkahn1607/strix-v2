---
trigger: glob
globs: "**/ann/**, **/memory/**,  **/worker/**"
description: "Concurrency contracts for the lock-free data plane: node seqlock-style versioning, hazard offsets, GC reclaimation, free list, ring buffers."
---

# Data plane concurrency contract

Loaded when touching `ann/`, `memory/` or `worker/`.

## Node control block

- 64-bit: 2-bit node state | 1-bit CLOCK reference bit | 4-bit seqlock version | 21-bit payload length |
  36-bit virtual offset.
- Snapshot ONCE before deciding the next behavior. Never re-read fields seperately for one decision.
  Modifying requires compare-and-swap, a direct `store` is viable once single ownership is guaranteed.
- Seqlock version advances only during re-acquisition by another worker - nothing else may bump it.
- Payload committing bounds-check node ID and requires PENDING note state.

## Reader protocol
 
- Worker reading payload follows the protocol: hazard publish -> revalidate -> copy -> postcheck -> clear.
- Revalidate right after hazard publishing from a single snapshot of node state, seqlock version and
  virtual offset.
- Post-validation of the copied bytes keys on seqlock version ONLY, never the node state. GC changing
  node state to DEAD can land mid-read and is not considered a corruption.
- On a successful read, promote reference bit COLD -> HOT via CAS (second-chance approximation of LRU).
- Payload bytes can tolerate ring-buffer wrap-around (two-chunk copy), but payload header does not.

## GC reclaimation (Snowplow)
 
- GC changing node state to DEAD may occur at any time, unguarded.
- Upon eviction, releasing node ID to the node free list and advancing read tail are ONE step, gated by
  a hazard-table check. Never split across GC iterations: splitting let a reader's payload region be
  reclaimed while its node ID was already back in circulation.
- Hazard-guarded reclaim has exactly two retry outcomes, whatever triggered it (COLD evict, forced evict
  or even payload rescuing):
  1. offset mismatch: rescue orphan, no node ID release;
  2. DEAD node state: deferred eviction, always releases node ID.
  Do not add a third path.

## Structures

- Data plane inherits gRPC internal thread pool, where each is given a "worker ID".
- Free list is a Treiber stack with tagged-pointer ABA protection. Never remove the tag.
- L0 ring buffer is an MPSC ring: acquiring workers are providers while background compaction is the
  sole consumer.
- Compaction / Reassignment stay on one background thread.
- Recalibrator is a resumable state machine: Idle -> Gathering -> K-means seeding -> Mini-batch -> Publish.
  The centroid pointer swap is atomic; the L1 IVF tables are double-buffered.

## Others

- Node clustering and vector / payload committing are "write-ahead publish-later" procedures. 

## Open questions

- The physical-buffer-reuse race was deferred pending a refcounted buffer pool. State whether
  Hazard offsets actually resolved it.
