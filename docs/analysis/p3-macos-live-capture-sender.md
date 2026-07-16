# P3 macOS bounded live capture sender

Task: `TASK-260712-26mnp1`

Status: engineering implementation; `live_ptt_v1` remains unadvertised and production-disabled.

## Implemented boundary

`MacLiveCaptureSender` opens the microphone only after a current local hold and
an authorized, validated `live_ptt_start` response agree on the same local
generation. A coordinator message cannot start or resume capture on its own.
Repeated press events are ignored while a generation is active. If the shell
cannot prove a release-capable hold input, the sender emits `fallbackToClip`
before opening the microphone so the existing toggle recording path stays the
safe fallback.

The sender consumes the Phase 1 capture backend's normalized 48 kHz mono
samples without using the durable WAV writer. A fixed 3,840-sample mailbox
passes capture data to a serial worker. The capture callback performs only a
non-waiting bounded offer and schedules at most one drain or overflow teardown;
encoding, framing, transport calls, metering and control messages stay off the
callback.

Frames are fixed at 960 samples/20 ms. The sender keeps one encoded lookbehind
frame so release can mark the final packet, and bounds the outbound queue at
eight frames. Backpressure, encoder failure or mailbox overflow terminate the
generation and stop the backend. No live samples are written to disk, logged,
placed in media repositories or retained after teardown.

## Lifecycle safety

One queue owns session, device, generation, sequence, timer and terminal state.
Release, local Stop, lost-release watchdog, maximum duration, system sleep,
session lock, permission revoke, device loss, app quit, disconnect and
backpressure converge on the same idempotent teardown. Teardown cancels the
timer, stops the backend, resets the encoder and fixed buffers, invalidates the
local generation and prevents delayed old release events from affecting a new
hold.

Visible and audible state are exposed as events for the later shell integration:
phase, local start request, meter, start cue, stop cue, terminal reason and clip
fallback. This task deliberately does not wire a global shortcut or advertise
the capability; `TASK-260712-2kj9kj` owns that integration.

## Codec boundary

The self-contained engineering encoder uses `AVAudioConverter` to produce raw
Opus access units at 48 kHz mono, 20 ms, 24 kbit/s constrained VBR. Repeated
system-encoder tests enforce the 400-byte packet ceiling. Apple does not expose
the frozen libopus complexity, expected-loss or in-band-FEC controls, so this
backend is not evidence for the exact production profile. The sender remains
dark until the reviewed libopus supply path and the other blockers in
[p3-live-codec-transport-adr.md](p3-live-codec-transport-adr.md) close.

## Deterministic evidence

Unit and source-inspection coverage proves:

- only a current local hold plus matching authorized start opens capture;
- invalid starts, key repeat and unavailable hold capability do not open it;
- packet order, timestamps, first/end flags and the 400-byte ceiling;
- bounded-queue saturation and device failure cannot leave capture active;
- 100 start/capture/release cycles are idempotent and reject stale releases;
- lost release, permission revoke, sleep, lock and disconnect stop the backend;
- the capture callback contains no encoder, network, filesystem, wait or
  blocking-lock work, and the sender contains no audio persistence client.

These are deterministic engineering checks, not physical proof of global-hold
behavior, real microphone/device transitions, sleep/lock delivery, audible cue
quality or 100 hardware cycles. That real-app evidence remains in
`TASK-260712-1rzqh9` under `EPIC-260714-th54l3`.

## Rollback and integration

No capability advertisement, websocket routing or shell behavior changes in
this task. Removing sender construction preserves existing clip capture.
`TASK-260712-2kj9kj` will connect the sender, receiver, transport and validated
hold seam after both platform halves exist.
