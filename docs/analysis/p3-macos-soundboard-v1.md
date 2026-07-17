# macOS soundboard and cue hotkeys v1

Status: best-effort engineering implementation for `TASK-260712-288j4a`.
Signed-app, audible playback, physical-keyboard, sleep/wake and permission-prompt
observations remain manual work in `EPIC-260714-th54l3` and are not claimed here.

## Canonical data and delivery

The macOS client uses the existing control-authorized `/v1/soundboard/cues`
CRUD/order API and `POST /v1/soundboard/cues/{cue_id}/trigger`. It does not
create a local delivery path. Manual triggers therefore retain coordinator
authority for cue lifecycle, target ACL, DND, Air policy, capability ceilings,
idempotency, downgrade rules, receipts and `mx_` execution lineage.

The native Soundboard section supports stable-ID cue selection, brokered audio
upload with an explicit rights acknowledgement, rename, reorder, delete,
route/delivery/include-origin preferences and manual triggering. Interrupt
fallback confirmation reuses the original idempotency key and only submits the
coordinator-minted token. A newly uploaded media item is best-effort deleted if
cue creation fails. Security-scoped file access ends when the upload attempt
finishes, and no local path or media ID is persisted.

Shared history now decodes display-safe manual/schedule/scoped automation
attribution and renders trigger, cue, principal, schedule, outcome, denial and
available control labels. The UI links to the automation administration area;
schedule editing, token administration and emergency mutations remain owned by
`TASK-260712-1oodka`.

## Hotkey and preference boundary

At most sixteen cue bindings are stored in a versioned, bounded UserDefaults
JSON value. The value contains only selected cue ID, route, delivery,
include-origin and cue-ID/shortcut pairs. It contains no bearer, media ID,
filename, path, audio bytes or microphone state.

Cue shortcuts use the existing Carbon `RegisterEventHotKey` boundary with
exclusive registration. Recording and cue registrars share a process-wide
monotonic ID allocator. A cue binding equal to the configured recording
shortcut is reported as a conflict before registration. OS conflicts and
unavailable registrations stay visible, while the window and menu-bar trigger
remain available. Sleep/session-inactive releases all cue registrations and
wake/session-active restores them. No event tap, global monitor, accessibility
permission or microphone workflow is used by the soundboard path.

## Automated evidence

Tests cover authenticated cue list/create wire shapes, display-safe automation
history, interrupt challenge decoding and confirmed replay, bounded secret-free
preferences, recording-shortcut conflict reporting, trigger lifecycle cleanup
and source-level absence of broad key monitoring and capture calls. `swift
build` is the local compile gate; the repository's Swift Testing suite runs in
hosted CI because the local command-line toolchain does not provide the
`Testing` module. Real-app and real-hardware observations stay manual.
