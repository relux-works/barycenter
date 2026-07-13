# Expose privacy-safe presence, DND and block controls

## Description
Project only useful delivery presence and enforce layered recipient controls without giving remote actors an emergency bypass.

## Scope
Expose online or offline, output ready or degraded, required capability support, current playback state and effective DND, with staleness timestamps and human pairwise-approach names. Implement frozen local-node and permitted orbit-level DND mutations so a remote command can never loosen a stricter local mode; support allow_all, messages_only and muted_until with coordinator clock handling. Implement role-scoped actor or orbit blocks, immediate scheduler visibility and exact missed_dnd versus blocked results. Never publish microphone state, capture state, process names, device names or raw peer IDs.

## Acceptance Criteria
Presence becomes stale or offline predictably from heartbeat data and leaks none of the forbidden local details. DND survives the specified lifecycle, expires muted_until correctly and cannot be remotely bypassed. Unauthorized block or DND mutation fails without disclosure; permitted changes affect future and contract-defined pending delivery consistently; app and bot render the same human state and exact receipt reason.
