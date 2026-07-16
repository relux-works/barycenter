# P3 Windows live PTT node integration

Task: `TASK-260712-2jbo5i`

Status: engineering integration; `live_ptt_v1` remains unadvertised and production-disabled.

## Integrated boundary

`WindowsLivePTTNode` serializes the reviewed Windows capture sender, jitter
receiver/audio route and frozen signalling around injected target-snapshot and
incoming-policy decisions. A local hold produces one sealed start request;
WASAPI capture opens only after a validated accept for the same session and
generation. Incoming starts are rejected while either direction is active, and
DND/block/policy authorization remains owned by the injected server-derived
state.

`WSClient` now accepts validated binary `BP` frames and exposes a non-blocking
binary sender. Both directions require an authenticated healthy connection and
an explicitly registered `live_ptt_v1` capability. Eight slots bound queued
plus in-flight binary writes. Validated live controls use 16 separate bounded
slots but the same FIFO, so `live_end` cannot overtake a final frame. Every item
is tied to the exact websocket pointer, so a frame queued for an old connection
is discarded after reconnect instead of crossing generations. The disconnect
callback provides the node teardown hook.

The status snapshot covers direction, phase, session/generation, accepted and
rejected receivers, terminal error and clip fallback for later tray/main-window
projection. Shipping `main.go` neither constructs the node nor advertises the
capability because the signed libopus and physical acceptance gates remain
open.

## Input and lifecycle safety

The node exposes button/menu/shortcut hold begin, heartbeat and release seams.
The existing AppContainer-compatible `RegisterHotKey` path produces one
`WM_HOTKEY` toggle notification; it does not prove global key-down and key-up.
This task therefore does not reinterpret it as a hold, install
`SetWindowsHookEx`, request broader input privileges or open capture when a
release-capable path is unavailable. The existing clip recording toggle stays
the visible fallback.

Release, local Stop, coordinator cancellation/failure, session lock, suspend,
permission revoke, device loss, disconnect/reconnect, feature rollback and quit
converge on sender and receiver teardown. Stale accepts and frames cannot start
or replace a current generation. Terminal control payloads remain validated by
the exact shared Go wire contract before transmission.

## Deterministic evidence

Go tests cover disabled fallback, matching versus stale accepts, concurrent
direction rejection, injected DND policy, binary buffering/playback routing,
generation-bound end, all lifecycle cleanup hooks, WebSocket capability and
health gating, malformed binary rejection, the eight-slot transport bound and
disconnect notification. Source guards keep the production capability,
composition and unreviewed keyboard hooks absent.

These checks prove bounded state and transport behavior, not packaged
AppContainer key-down/up, signed microphone/output behavior, audible ducking,
two-home interoperability, sleep/lock delivery or physical device recovery.
Those checks remain in `TASK-260712-1rzqh9` under `EPIC-260714-th54l3`.

## Activation and rollback

Activation still requires a reviewed signed libopus supply path, exact codec
profile evidence, authoritative target/DND composition, a supported
release-capable input and the manual two-home matrix. Until then, registration
and construction must stay absent. Rollback removes the future composition and
capability; Phase 1/2 playback and clip capture remain unchanged.
