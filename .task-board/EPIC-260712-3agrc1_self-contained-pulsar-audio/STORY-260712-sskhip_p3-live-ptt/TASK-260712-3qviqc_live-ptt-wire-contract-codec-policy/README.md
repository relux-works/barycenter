# Freeze the generation-safe live PTT protocol

## Description
Define bounded signalling and binary media envelopes across Go, Windows and Swift only after input and transport spikes pass.

## Scope
Add live_ptt_v1 capability and start, accept or reject, binary chunk, end, cancel, failure, receipt and state semantics with random session ID, monotonically increasing generation and sequence, sender and frozen target context, codec profile, timestamp or duration and strict maximum lengths or rates. Freeze one active live speaker per playback domain, Air policy, DND or block, unsupported-target and toggle fallback behavior, late join or live-edge rule, drop or retransmit policy, duplicate or reorder handling, end or drain timeout and coordinator restart non-resume. Add docs, golden signalling JSON and binary vectors, Go, Windows and Swift codecs and malformed-frame tests while preserving clip and track protocols.

## Acceptance Criteria
All mirrors accept the same bounded valid envelopes and reject truncated, oversized, wrong-profile, stale-generation, duplicate and unauthorized media. Reconnect or restart cannot attach to an old session, unsupported targets are explicit, fallback remains a Phase 1 toggle clip rather than hidden live behavior, and no remote message can start a microphone.
