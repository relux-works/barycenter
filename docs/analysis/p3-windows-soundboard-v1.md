# Windows soundboard, hotkeys and manual delivery v1

Status: engineering contract implemented by
`TASK-260712-1yw7fo`. Real Windows installation, audible playback, signed MSIX
and physical-keyboard observations remain in the manual-testing epic
`EPIC-260714-th54l3`; this document makes no such claim.

## Canonical manual trigger

`POST /v1/soundboard/cues/{cue_id}/trigger` is the control-authorized manual
entry point. It accepts the ordinary audience, delivery, include-origin and
interrupt fallback-confirmation shape and returns both `mx_...` execution and
`tr_...` transmission IDs. Manual soundboard use requires
`soundboard_enabled`, but deliberately remains available while scheduled or
scoped automation is disabled.

The store resolves the active cue and its current source inside the same
transaction that creates the ordinary transmission. Target ACL, block state,
DND, Air policy, capability ceilings, delivery downgrade, idempotency and
receipts therefore have one authority. Builtin cue bytes are materialized at
the canonical per-orbit storage key and published as ordinary ready media;
authorized user cues retain their existing owned media lifecycle. Deleted,
revoked or stale cue state fails closed.

Accepted manual delivery adds immutable `manual_soundboard_executions`
lineage and an append-only audit event. Canonical `/v1/history` reconciles its
current transmission status and reason, while exposing only the display-safe
cue, audience, count and execution attribution. It never projects a bearer,
idempotency digest, raw selector, private path or storage key.

## Windows shell and tray

The native window has a dedicated Soundboard section. It supports:

- builtin and authorized media cue selection;
- brokered file creation with per-upload rights confirmation;
- rename, move-down reorder and delete using revision-checked control calls;
- `this_pulsar`, `own_barycenter` and `current_air` routing;
- overlay, interrupt and after-current delivery plus include-origin;
- manual button trigger and explicit interrupt fallback confirmation;
- navigation to the downstream automation administration surface;
- display-safe automation attribution, terminal reason and available
  schedule/principal/emergency quick-control labels from shared history.

The tray provides an always-visible trigger for the selected cue. Neither
window nor tray requires Telegram or an external account beyond the active
Pulsar control identity.

Brokered file capabilities are opened once, copied to app-private staging,
uploaded through the canonical media API and deleted locally after confirmed
upload. A failed cue creation best-effort deletes the newly uploaded remote
media. Preferences never persist the brokered path or media ID.

## Hotkey and preference boundary

At most sixteen cue bindings are stored. The versioned JSON contains only the
selected cue ID, route, delivery, include-origin and bounded
cue-ID/shortcut pairs. It is atomically replaced with owner-only permissions.
It contains no control token, media ID, local path, audio bytes or microphone
state.

Production uses `RegisterHotKey` with `MOD_NOREPEAT` on the existing
message-only tray owner. Registration IDs come from a process-wide atomic
allocator so recording and cue controllers cannot alias. Each cue reports
`registered`, `conflict`, `unavailable` or `suspended`; a recording-shortcut
collision is rejected before the OS call. Session lock and system suspend
release every registration, and resume re-registers only after all suspension
reasons clear.

No keyboard hook, key logger or capture API is used. A failed hotkey never
disables the window/tray button path, and a soundboard action never calls the
microphone workflow.

## Automated evidence

Tests cover the manual trigger transaction, exact replay lineage, shared
history, automation-disabled/manual-enabled behavior, HTTP auth and response
redaction, Windows cue CRUD/trigger decoding, brokered staging cleanup,
bounded secret-free preferences, recording/system conflicts, lifecycle
cleanup, button fallback, interrupt confirmation key reuse, automation history
projection and absence of hook/capture symbols. Full coordinator and Windows
client suites, vet, focused race tests and Windows cross-compilation are the
engineering gates. Physical Windows behavior remains manual evidence.
