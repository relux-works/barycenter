# P3 Windows live capture sender

Status: accepted engineering candidate for `TASK-260712-ezdhpf`.

## Boundary

`WindowsLiveCaptureSender` binds an accepted coordinator session to an exact,
current local hold generation. A signal received without that local state,
with a stale generation, without authorization, or with an invalid frozen
`live_ptt_v1` payload cannot request permission or open the microphone. When a
store-safe hold surface is unavailable, the sender emits only the existing
clip fallback event.

The sender uses the Phase 1 AppCapability and WASAPI backend directly and does
not use `WindowsMicrophoneCaptureService`, `CaptureMediaStore`, a temporary
file, or any other persistence path. It emits phase, meter and cue events for
the later node-integration task; this task does not advertise the production
capability or wire a UI surface.

## Bounded path

- WASAPI is read into one fixed 2,048-frame buffer. Input is validated as
  8--192 kHz and 1--32 channels, mixed to mono, clamped and resampled into a
  fixed-capacity worker buffer.
- Encoding runs off the native capture boundary in exact 960-sample, 48 kHz
  mono frames. Payloads are rejected outside 1--400 bytes and every structured
  binary frame is revalidated against the frozen wire encoder.
- Sequence timestamps advance by exactly 20 ms. The first frame carries
  `START`, every frame carries the frozen FEC bit, and a one-frame lookbehind
  places `END` only on the final frame.
- Capture can enqueue at most eight frames to a dedicated transport worker.
  The capture worker never calls transport, control signalling, disk, or
  logging code. A full queue stops the microphone and emits a validated
  discard-buffered backpressure cancel.
- A normal release closes WASAPI before terminal transport drain. The sender
  remains `stopping` and rejects a new hold until the final frame and validated
  `live_ptt_end` are ordered, or the fixed 600 ms drain expires and becomes a
  validated backpressure cancel.

## Lifecycle

Generation invalidation, a 1.5 s lost-release watchdog, and the five-minute
wire maximum prevent key repeat, a missing key-up, or an old callback from
resuming capture. Release, local Stop, lock, suspend, permission revoke, device
loss, quit, disconnect, coordinator cancellation, encoder failure and queue
saturation converge on one stream stop/close path. Disconnect, quit and an
already-authoritative coordinator cancel do not attempt a terminal network
write. Other terminal payloads are validated before the injected control send.

Only aggregate frame/byte counters and an RMS meter leave the worker. PCM and
encoded payloads are held only in fixed or bounded ephemeral memory and are
released at terminal cleanup; samples are never logged.

## Codec and evidence boundary

The accepted codec spike selected libopus 1.6.1 with 48 kHz mono, 20 ms,
24 kbit/s constrained VBR, complexity 5 and FEC/PLC. There is still no reviewed
signed Windows libopus supply path in the CGO-free packaged application.
Consequently this change exposes only an injected `WindowsLiveOpusEncoder`
seam, registers no production encoder, and leaves `live_ptt_v1` unadvertised.
This is the same fail-closed supply-chain boundary recorded in
`p3-live-codec-transport-adr.md`.

Portable tests prove unsolicited/stale/unauthorized rejection, fallback before
microphone access, exact framing, 8 kHz resampling, hard payload/queue bounds,
100 start/stop cycles, stale releases, native device failure, permission loss,
lock, suspend, disconnect, lost release, local Stop, maximum duration, encoder
failure, saturation and non-overlapping terminal drain. Source inspection
guards the capture worker from transport, persistence and sample logging.

Real signed Windows 10/11 hold input, microphone routes, lock/suspend/device
events, audible cues, intelligibility, latency and repeated hardware cycles are
not claimed here. They remain manual evidence in `TASK-260712-1rzqh9` under
`EPIC-260714-th54l3`.
