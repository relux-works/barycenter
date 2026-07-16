# P3 live PTT wire contract

Status: engineering contract frozen for `TASK-260712-3qviqc`; production capability remains disabled pending runtime implementation and physical C2 evidence in `TASK-260712-1rzqh9`.

## Decision

`live_ptt_v1` is an additive protocol-v1 capability. It uses the existing authenticated WebSocket for eight JSON signals and a bounded binary Opus frame. A node MUST NOT advertise the capability until its complete capture or receive runtime is ready. Unknown signals retain protocol-v1 rejection behavior, so clip and streamed-track clients do not change.

The signalling catalog is `live_ptt_start`, `live_ptt_accept`, `live_ptt_reject`, `live_ptt_end`, `live_ptt_cancel`, `live_ptt_failed`, `live_ptt_receipt`, and `live_ptt_state`. Exact fields and bounds are represented by the typed Go, Windows, and Swift mirrors and their JSON goldens. `protocol/live-ptt-v1.json` is the machine-readable policy contract.

## Identity, ordering, and lifecycle

- Every attempt gets a cryptographically random, non-zero 128-bit session ID encoded as 32 lowercase hex characters. Generation increases monotonically in its playback domain; frame sequence starts at one.
- The target set and digest are frozen by `live_ptt_start`. Late joiners never auto-join. DND, block, membership, Air ownership, and one-active-speaker rules are checked before accept and continuously; revocation cancels and discards buffered audio.
- The first binary frame has `start`; the terminal frame has `end`. Exact repeated frames are idempotent. Older/reordered frames are stale. A forward gap of at most eight frames is accepted for FEC/PLC; a larger or timestamp-inconsistent gap is invalid. Frames are never retransmitted.
- Accept must precede media and arrive within 1500 ms. The receiver starts at sequence one after a three-frame/60 ms jitter buffer. End drains for at most 600 ms. Sessions last at most five minutes and have no persisted media.
- Disconnect, coordinator restart, client restart, lock, sleep, permission loss, device loss, or lost release terminate the generation. A reconnect cannot resume it; a fresh local hold creates a new session and generation.

## Binary frame

The header is 40 bytes, big-endian: `BP` magic (2), version (1), flags (1), session ID (16), sequence (4), capture monotonic microseconds (8), payload length (2), frame duration (1), channels (1), sample-rate profile (1), codec profile (1), and reserved zero (2). The payload is 1–400 bytes. The frozen profile is Opus 1.6.1, 48 kHz mono, 20 ms, 24 kbps constrained VBR, complexity 5, in-band FEC at the 2% profile. Profile bytes are `20,1,1,1`; unknown flags and non-zero reserved bytes are rejected.

## Authority and mixed versions

Capture authority is always `local_user_input_only`: no received message can start a microphone. There is no automatic conversion to a clip. `require_all` rejects before start when any frozen target is unsupported. `supported_only_with_receipts` proceeds only to supported targets and records explicit terminal `unsupported` receipts for the rest. The UI may separately offer the existing Phase-1 toggle-to-record-a-clip action, which creates a different operation.

## Verification and rollout boundary

The coordinator and Windows packages share byte-equal Go sources. Swift has an independent codec. All three consume the same 59 JSON goldens; binary vectors cover valid start/middle/end plus truncated, oversized, wrong-version, wrong-profile, bad-flags, bad-reserved, zero-sequence, and length-mismatch inputs. Generation guards cover duplicates, stale frames, gaps, terminal state, and post-terminal rejection.

This task freezes only the wire boundary. It does not register a decoder, advertise `live_ptt_v1`, open a microphone, or make the transport production-ready. Those actions belong to subsequent runtime and platform tasks. Physical Windows/macOS latency, signed-package, sleep/lock/device-loss, hostile-input, and intelligibility evidence remains in the manual-testing epic.
