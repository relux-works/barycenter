# Prototype canonical variants, authenticated ranges and bounded cache

## Description
Freeze the server and node transport substrate all decoder candidates consume, including random-seek and revocation behavior.

## Scope
Compare original upload with one or more canonical compressed variants and prototype stream_variants metadata, codec or container, bitrate, duration, size, ETag and integrity or chunk-manifest fields plus VBR seek mapping. Define authenticated byte-range semantics including Accept-Ranges, 206, Content-Range, Content-Length, 416, conditional or If-Range behavior and uniform ACL failure; authorize every request through immutable target snapshots and prevent shared-cache tenant leaks. Define app-private bounded disk cache ceilings, atomic chunks, LRU or pinning, concurrent readers, restart reuse, eviction, corruption recovery and immediate delete or actor-disable invalidation without full RAM download.

## Acceptance Criteria
One reviewed contract supports prepare and arbitrary seek by ranges for every candidate, with stable RFC-compatible headers, no bearer in URLs, target ACL on every fetch and no cross-tenant cache oracle. Integrity detects corrupt or mixed versions before decode, VBR seeking is deterministic, cache disk and memory are hard-bounded, and revoke or delete prevents refill and removes or invalidates cached access according to policy.
