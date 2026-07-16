# P3 bounded ephemeral live PTT coordinator runtime

Status: engineering implementation for `TASK-260712-3vzbbl`. The runtime is dark by default and does not make `live_ptt_v1` production-ready.

## Runtime boundary

The coordinator accepts live signalling only when `DUET_LIVE_PTT=1`. The flag is environment-only so the pinned rollback coordinator can consume the same YAML. Nodes still must not advertise `live_ptt_v1` until their later platform runtime tasks are accepted.

An authenticated source socket is re-resolved to its current actor, membership and installation binding for every start. The Store resolves the current barycenter/Air target set in one transaction, applies Air overlay policy, recipient block and DND, and matches every online target to the exact authenticated socket generation and capability set. The coordinator replaces all caller-supplied target metadata with a canonical sealed-target digest and count.

The coordinator also replaces the proposed session identity with a new CSPRNG 128-bit ID and a new monotonic runtime generation. A reconnect is treated as a disconnect before registration completes. Coordinator shutdown drops the in-memory manager. Consequently, replaying a former start or binary frame cannot attach to the former generation; a new accepted operation always receives a new identity.

## Bounded fanout

The WebSocket Hub now has one ordered outbound queue per connection for JSON and binary frames. Each queue is fixed at 32 entries. JSON control messages retain their existing reliable blocking behavior. Live binary enqueue is non-blocking: a full or absent target queue fails only that target and produces metadata-only failure effects; it never waits on or grows memory for a slow receiver.

Inbound binary messages are limited by the existing 64 KiB WebSocket read cap and then by the frozen 40–440 byte live frame decoder. Wrong magic/version/profile/flags/reserved fields, invalid sequence/timestamp, truncated and oversized frames are rejected before entering the coordinator loop. The Hub checks the current socket generation again before emitting a binary event. Neither inbound nor outbound paths format payload bytes into logs.

The session manager keeps only sender/domain/session identity, target states, deadlines, counters and the last frame digest required for exact-duplicate detection. It returns the current bounded frame directly to per-target sends and does not retain it after the call. It has no Store/media dependency. A global 256-session ceiling, the one-session-per-domain rule and a per-session 50 frame/s token bucket with an eight-frame burst cap bound active metadata and relay work. Its public metrics keep `retained_audio_bytes` and `persisted_audio_bytes` at zero.

## State and failure rules

- There is exactly one winner per playback domain. A concurrent start receives `busy`.
- Targets are frozen at start. `require_all` rejects if any target is unavailable; `supported_only_with_receipts` starts with capable targets and emits explicit `unsupported` or `rejected` metadata receipts for excluded targets.
- Media is invalid until a target accepts. The first acceptance opens the duck boundary. Exact frame duplicates are no-ops; stale, wrong-sender, post-terminal and invalid-gap frames fail closed.
- A slow/offline/revoked target becomes terminal independently. Remaining accepted targets continue. Loss of the last target terminates the session and releases duck.
- Existing durable overlay or interrupt work wins before a live start. Once live has won, the durable scheduler leaves that playback domain queued until the live duck/release boundary ends, then wakes immediately. This serializes the two runtimes without writing live chunks or converting them into transmission rows.
- Sender release sends bounded end/drain control. Cancel, sender disconnect, target policy loss, accept watchdog, five-minute watchdog, replacement connection and coordinator restart terminate without resume.
- DND changes conservatively disconnect the affected live node generation from active live sessions. Ordinary Phase 1 clip and Phase 2 track paths are not mutated.

## Telemetry and privacy

`/healthz` exposes only whether the env gate is enabled, active-session count, total relayed frames, total dropped targets and retained audio bytes. Logs contain session IDs, node coordinates, reason codes and duck boundaries, never chunks. No `media_items`, `transmissions`, upload, history or ordinary event rows are created for live audio.

## Verification and rollback

Focused tests cover authenticated sealed-target resolution, DND, stale socket binding, deterministic winner/busy behavior, pre-accept rejection without consuming sequence state, duplicate/stale frames, the bounded relay rate, isolated per-target backpressure, continuous capability withdrawal, durable overlay serialization and wake-up, sender disconnect, watchdog, restart non-resume, binary Hub ingress/egress, env-off rejection, health privacy and full loop fanout/end recovery. The repository acceptance suite additionally protects rollback, clip/track behavior, Windows mirrors and Swift protocol compatibility.

Rollback is immediate: unset `DUET_LIVE_PTT` and restart. Existing nodes do not advertise the capability, existing clients receive explicit `unsupported`, and the predecessor-neutral YAML and database remain valid. Physical latency, audio recovery, signed application behavior, hostile encoded input and intelligibility are still manual evidence in `TASK-260712-1rzqh9`.
