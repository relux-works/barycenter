# P3 macOS bounded live jitter receiver

Task: `TASK-260712-19w1qn`

Status: engineering implementation; `live_ptt_v1` remains unadvertised and production-disabled.

## Implemented boundary

`MacLiveJitterReceiver` validates the frozen `live_ptt_v1` start contract,
authorization, monotonic generation, 128-bit session identity, sequence, flags,
capture timestamp and codec profile before decoding. It accepts one active
session, keeps at most nine encoded frames (3,600 payload bytes), starts at a
three-frame/60 ms live edge and rejects stale, duplicate-conflicting,
out-of-window and post-session frames.

Packet, decoder, timer and event work is serialized on a dedicated queue. PCM
is written to a fixed 15,360-frame/320 ms SPSC ring. The audio callback only
reads that preallocated ring, zero-fills an underrun, applies a render-owned
raised-cosine gain and mixes the 48 kHz mono branch before the existing common
DynamicsProcessor limiter and final local master gain. It performs no decoder,
network, filesystem, queue, wait or allocation work.

The receiver pre-ducks the music branch by 12 dB over 60 ms. Live activation
ramps in over 5 ms. End drains already-decoded PCM within the frozen 600 ms
deadline; cancel, DND/policy revoke, disconnect, timeout, decoder failure or
ring overflow discard buffered live PCM. Terminal gain release and music
recovery use bounded ramps and generation tokens, so a delayed old cleanup
cannot clear a replacement session.

## Loss recovery

The decoder seam supports one-frame Opus in-band FEC when a reviewed backend
implements it and the following packet is present. The self-contained macOS
backend uses `AVAudioConverter` with raw Opus access units for ordinary decode.
Apple's API does not expose `decode_fec`, so this backend explicitly returns
`fecUnavailable`; the receiver then uses a bounded, attenuated waveform PLC
frame and never waits past the current playout point. Eight consecutive
concealments are the hard limit before terminal failure.

This is intentionally not represented as production FEC evidence. Staging and
reviewing the frozen libopus binary remains one of the production blockers in
the codec ADR.

## Deterministic evidence

Unit coverage proves:

- authorization, second-session and stale-generation rejection;
- in-window reorder, exact duplicate handling, late drop and injected FEC;
- 100 scheduled frames with exactly two absent packets using bounded PLC,
  without PCM or encoded-window growth, stall or terminal failure;
- hard PCM overflow failure, drain-before-end and discard-on-cancel cleanup;
- raw Opus packet decode through the system AudioConverter into one fixed
  960-sample frame;
- source-inspected render safety and graph ordering through the common limiter.

The 2% case proves deterministic state and memory behavior, not speech
intelligibility, calibrated latency, audible PLC quality or absence of clicks
on physical output. Those Windows/macOS two-home and real-device checks remain
in `TASK-260712-1rzqh9` under `EPIC-260714-th54l3`.

## Rollback and integration

No capability advertisement or websocket command wiring changes in this task.
Removing the receiver construction leaves the existing graph and protocol-dark
behavior intact. `TASK-260712-2kj9kj` owns macOS node websocket/capture/session
integration after both sender and receiver paths exist.
