# Phase 2 macOS targets and inbox UI

- Date: 2026-07-16
- Task: `TASK-260712-2nto40`
- Portable model: `pulsar.targets-inbox-presentation.v1`
- Coordinator contract: `p2-targets-inbox-parity.v1`

The native macOS main window and menu bar now expose the shared Phase 2 target,
inbox and delivery-history projection. The UI renders only coordinator-owned
localized labels and opaque expiring target capabilities; it never renders an
actor, orbit, slot, binding, cursor or target reference.

## Runtime boundary

`TargetsInboxAppClient` binds the control credential to the existing target,
inbox, history, receipt, content-policy and moderation endpoints. It enforces
same-origin responses, bounded JSON, exact contracts and typed IDs before the
data reaches SwiftUI. `MacTargetsInboxAppComposition` is the only native
executor for `PulsarTargetsInboxCommand` values. It:

- serializes one refresh or mutation at a time;
- rejects commands that no longer match the current ready capability model;
- keeps retained rows visible but disables mutations while stale or offline;
- deduplicates appended pages and restarts at page one after `cursor_expired`;
- refreshes authority after replay, dismiss, delete, report or mute; and
- clears the Phase 2 projection when pairing/core ownership is torn down.

The existing durable Phase 1 draft outbox remains a separate owner. A Phase 2
network failure cannot delete or silently promote a local draft. Explicit clip
sends pass one to 64 current `trf_` capabilities plus `include_origin` through
the common `POST /v1/transmissions` boundary. The outbox durably freezes the
sorted capabilities and idempotency key before upload/transmit work, protects
that local record with the existing owner-only file policy and retries the exact
intent without another upload. References are never rendered or logged.

Versioned content-policy state is shown in this surface; the established
exact-manifest acceptance flow still runs at send time. Track queue/replace
stays visibly unavailable until the later streamed-track story supplies
`audio_track_v1`, `queue_replace_v1` and a reviewed player path. The UI does not
claim a fallback.

## Native interaction and no autoplay

The `Inbox & targets` sidebar/menu command provides audience, explicit permitted
target and include-origin controls, capability state, TTL, requested/effective
delivery, exact receipts, pagination and capability-filtered moderation. Lists
use stable server IDs and native `Button`, `Toggle`, `Picker`, confirmation and
focus-section semantics. A ready draft can be sent to the current explicit
selection; a retry uses only its already-frozen target intent. English and
Russian copy is selected without exposing wire enums.

There is no read-time playback hook. Inbox load, pagination, reconnect and menu
opening call refresh only. Playback can start only when the user activates the
Replay button and the current ready model still returns a replay command under a
current content-policy grant.

## Automated and manual evidence boundary

The Xcode Swift suite covers authenticated wire requests, content-policy-required
state, explicit replay versus read paths, model capability checks, opaque-value
redaction, EN/RU source seams, stable keyboard commands and absence of
`onAppear`/tap autoplay. App compilation covers the complete AppKit/SwiftUI
composition.

No live VoiceOver session, packaged-app keyboard traversal, audible replay,
multi-node reconnect, physical Mac or mixed-fleet result is claimed here. Those
hands-on B5-B7 observations remain in manual epic `EPIC-260714-th54l3`, primarily
`TASK-260712-3u5cdn`.
