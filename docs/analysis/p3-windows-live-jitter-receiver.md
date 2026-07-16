# P3 Windows bounded live jitter receiver

Task: `TASK-260712-1ckdr7`

Status: engineering implementation; `live_ptt_v1` remains unadvertised and production-disabled.

## Implemented boundary

`WindowsLiveJitterReceiver` validates the frozen start profile, authorization,
accept deadline, monotonic generation and one-session ownership before it
prepares audio. Binary frames must carry the active 128-bit session ID, exact
flags, a bounded sequence, the fixed 20 ms timestamp progression and a payload
of at most 400 bytes. The receiver keeps at most nine encoded packets and
rejects malformed, conflicting, stale and out-of-window data before decode.

Packet, timer, decoder and control-message work is mutex-serialized outside the
WASAPI callback. Three decoded frames establish the negotiated 60 ms live edge.
The decoder seam supports one-frame in-band FEC; when the reviewed backend
reports FEC unavailable, the receiver produces a bounded attenuated waveform
PLC frame. More than eight consecutive concealments is terminal rather than an
unbounded silent stall.

Decoded 48 kHz mono frames are linearly converted off render into fixed 44.1
kHz stereo frames for the existing Windows engine. Each render generation owns
a 320 ms SPSC PCM ring. The engine rejects old-generation writes, and an epoch
check prevents a callback holding an old state pointer from mixing a
replacement generation.

## Mixer and lifecycle

The live branch pre-ducks the main program by 12 dB over 60 ms, ramps live PCM
in over 5 ms and mixes before the existing post-mix limiter and local master
gain. The common limiter pins the active live mix to -1 dBFS. Render reads only
the fixed ring and preallocated scratch; it performs no decode, network,
filesystem, goroutine creation, allocation, blocking lock, wait or sleep.
The first successful ring read posts a fixed completion to the existing
off-render dispatcher; only that dispatcher emits `audible_started`, so buffer
publication alone cannot claim audible playback.

Normal end drains decoded PCM within the frozen 600 ms deadline. Cancel,
policy/DND/leave/disable revoke, coordinator loss, decoder failure, excessive
concealment, maximum duration and PCM overflow discard buffered live data. A
5 ms live tail and 160 ms duck release are render-owned; generation checks keep
late cleanup from clearing a replacement session. Terminal receiver state and
the decoder are reset immediately while the bounded audible release completes.

## Decoder boundary

This task intentionally exposes `WindowsLiveOpusDecoder` but registers no
production implementation. The accepted codec investigation found that inbox
Media Foundation rejects the required Ogg/Opus inputs, while no reviewed,
signed libopus binary is staged in the CGO-free Windows package. Test decoders
exercise ordinary decode, injected FEC and explicit FEC-unavailable PLC without
misrepresenting those doubles as a shippable codec.

The production no-go in
[p3-live-codec-transport-adr.md](p3-live-codec-transport-adr.md) therefore
remains unchanged. `TASK-260712-2jbo5i` may wire the dark integration seam,
but it must not construct a production decoder or advertise the capability
until an accepted decoder supply exists merely because this bounded runtime
compiles.

## Deterministic evidence

Automated coverage proves:

- authorization, concurrent-session and stale-generation rejection;
- exact frame/profile/timestamp validation, duplicate/conflict handling and a
  nine-packet reorder ceiling;
- 100 frames with exactly two absent packets recovered through injected FEC,
  with bounded encoded and PCM storage and no render stall;
- explicit PLC fallback, eight-concealment cutoff and PCM overflow failure;
- 60 ms activation, common limiter ceiling, normal drain, cancel/revoke fade,
  duck recovery and old-generation rejection;
- maximum-duration cleanup and fixed-storage source inspection of both the
  receiver and WASAPI render boundary.

The 2% test is deterministic state/memory evidence, not real speech
intelligibility, calibrated latency, audible PLC quality, click inspection or
signed-AppContainer playback. Those Windows/macOS two-home and physical-device
checks remain in `TASK-260712-1rzqh9` under `EPIC-260714-th54l3`.

## Rollback

The receiver is not constructed by the production composition and no node
capability changes. Removing the receiver and live engine seam leaves existing
music, voice, overlay, interrupt and click paths unchanged.
