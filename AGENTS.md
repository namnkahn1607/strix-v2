# Strix - agent contract

Strix is a high-performance semantic cache: control plane (HTTP gateway, supervisor, CLI)
written in Golang at `controlplane/` and data plane (embedded vectorization model,
lock-free vector index, payload caching/storage) written in C++ at `engine/`, talking
gRPC over Unix domain socket.
Concurrency correctness outranks everything else in this file.

## How to work here

- If a change touches a concurrency protocol (see `.agents/rules/data-plane.md`),
  name the invariant it affects before editing.
- Smallest diff that solves the problem. Do not rename, reformat, or refactor code you were
  not asked to touch.
- Do not invent APIs, flags, or CI job names. Grep first. If unsure, say so.

## Architecture (stable facts)

- 3 OS processes in Star topology: Supervisor (CLI + process management) forks Gateway (Golang)
  and Engine (C++) as child processes. Death pipes + blocking `read()` give ordered shutdown,
  no zombies.
- Control plane (Golang) <-> data plane (C++): gRPC/Protobuf over UDS (`CheckCache` / `SetCache`).
  Full contract at `proto/v1/cache.proto`.
- Data plane layers: unified Memory Arena (node array, vector array, payload ring buffer) -> two-tier
  vector storage (Level 0 ring buffer and Level 1 IVF) -> CLOCK-based "Snowplow" garbage collector.

## Decisions already made (do not re-propose)

- Always normalize output embedding vector from embedded inference model, as the dot product kernel
  use L2-normalized vectors.
- Hazard offsets (not EBR) protects payload reads from GC eviction. Reclaim of the ring buffer /
  bump allocator needs per-range precision that EBR's global epoch quiescence cannot give.
- Snowplow GC, not Copy-Forward (Copy-Forward had fundamental concurrency issues) runs on a dedicated
  background thread.
- IVF coordinator does node compaction/migration from L0 to L1, reassignment (fix cluster datapoints)
  and recalibrates IVF centroid tree also runs on a dedicated background thread.
- Development and production both targets Ubuntu (from 24.04 to latest 26.04).

## Conventions (when writing code)

- Comments explain why, or state a non-obvious invariant or memory ordering justification but not
  restate what the code does. No banner or section comments. Do not rewrite an existing comment unless
  the code behavior it describe changes.

## Accepted limitations (do not "fix" unless asked)

- Approximate-LFU rate-limiter eviction can evict a legitimate first-time IP when a shard is full
  under DDoS.
- No C++ sanitizer instrumentation on third-party dependency builds.
