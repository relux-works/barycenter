# Phase 2 Windows targets and inbox UI

- Date: 2026-07-16
- Task: `TASK-260712-cuplon`
- Portable model: `pulsar.targets-inbox-presentation.v1`
- Coordinator contract: `p2-targets-inbox-parity.v1`

The packaged Win32 main window now has an `Inbox & targets` section backed by
the same server-owned projection and command vocabulary as macOS. It renders
only localized presentation labels. Target references, inbox/history IDs,
cursors, actor IDs, slots and credentials remain inside the typed client and
composition and are excluded from labels and logs.

## Runtime and retry boundary

`TargetsInboxAppClient` reuses the hardened active-control transport and binds
the existing targets, inbox, history, receipt, policy and moderation endpoints
to bounded typed responses. It rejects redirects, malformed or duplicate
opaque capabilities, mismatched contracts/action arrays and non-canonical IDs.
Projection reads have no media/download endpoint.

`WindowsTargetsInboxComposition` serializes refreshes and mutations, validates
every action through `TargetsInboxModel.BuildCommand`, deduplicates pagination,
restarts from the first page after `cursor_expired`, and refreshes authority
after mutation. Stale/offline rows remain readable while selections and action
authority are removed. An automatic refresh does not remint a current explicit
selection; a manual or post-mutation refresh preserves it only when the new
authoritative response still contains the exact references.

The durable Windows draft outbox now freezes a sorted one-to-64-reference
explicit audience, `include_origin`, delivery and the existing upload/transmit
idempotency keys before network work. A restart retry uses that exact protected
record, performs no second upload and rejects any changed audience. The shell
shows the frozen recipient count but never the references.

Versioned Terms/content-policy acceptance remains separate from the per-upload
rights confirmation and is completed before send. Queue/replace is visibly
unavailable until the streamed-track story supplies its reviewed contract and
player; there is no clip fallback pretending to satisfy it.

## Native interaction and no autoplay

The section provides native tab-stop buttons/edit controls for audience,
permitted targets, include-origin, delivery, exact send/retry, inbox/history
pagination, receipts, replay, dismiss, delete, report and mute. `Ctrl+4` opens
the section and `Ctrl+R` refreshes it. Controls are laid out in device-independent
pixels under the existing PerMonitorV2 manifest and reflow on `WM_DPICHANGED`.
English and Russian text plus textual `[~]`, `[+]` and `[!]` state markers avoid
color-only meaning.

Inbox projection, pagination, reconnect and window refresh never call replay.
Playback can begin only through the explicit Replay button while the current
ready capability model still allows replay and the policy grant is current.
Permanent history deletion requires a native confirmation.

## Automated and manual evidence boundary

Portable, race and Windows cross-build tests cover authenticated endpoint/body
shape, duplicate-capability rejection, initial and paginated receipts, exact
durable retry, stale authority removal, absence of refresh-time replay, EN/RU
opaque-value redaction, keyboard uniqueness, Win32 source wiring and 96/120/144/
192-DPI reachability. The packaged probe compiles the production localized MSIX
schema without treating that as a real-device result.

No live Narrator/screen-reader session, packaged-app tab traversal, audible
replay, multi-node reconnect, physical Windows PC or mixed-fleet result is
claimed here. Those hands-on B5-B7 observations remain in manual epic
`EPIC-260714-th54l3`, primarily `TASK-260712-3u5cdn`.
