# Render-safe mixer control contract

`TASK-260712-1hqiek` freezes the control/render boundary shared by the macOS
and Windows transmission clients. Later overlay, interrupt, streaming and live
tasks extend the mixer behind this boundary instead of introducing a second
lifecycle.

## Immutable control carrier

Every prepared clip carries `MixerControlParameters`: transmission ID,
generation, delivery ID, duck level and attack/release, interrupt fade-out and
fade-in, limiter ceiling, interrupt permission, and started/ended telemetry
flags. Both clients derive this carrier from the same wire payload defaults:
non-positive duck dB, non-negative fades, limiter ceiling `-1 dBFS`. The prepared media
buffer and carrier are created before arming playback.

The client lifecycle is `prepared -> armed -> playing -> cancelling ->
terminal`. A `(transmission_id, generation)` owns the state. Duplicate or older
commands are ignored. A newer prepare cannot replace active armed, playing, or
cancelling audio. A newer sender-delete first asks the mixer to stop the active
buffer; only its acknowledgement can publish the newer terminal tombstone.
Late started/ended callbacks are checked against the owning generation and
cannot resurrect terminal state. The first terminal reason is frozen.

## Render boundary

All file reads, authenticated HTTP, decoding, `AVAudioPCMBuffer` allocation,
slice construction and command construction occur on control/preparation paths.

- macOS publishes fixed-size gain commands through a preallocated 64-entry SPSC
  queue. The source callback owns ramp mutation and uses fixed scratch storage,
  the lock-free PCM ring and C11 atomic counters. First-sample notification is
  handed to a pre-created dispatcher by an atomic host-time slot.
- Windows publishes immutable voice and click snapshots with atomics. The
  callback owns cursors, reads atomic gain/telemetry state, and posts completion
  to a pre-created bounded dispatcher. Music gain commands and the master
  amplitude are atomic on the render side.

Source-safety tests reject allocation primitives, goroutine creation, waits and
blocking lock calls inside the checked render functions. Full Windows race
tests exercise the handoff concurrently.

## Legacy voice compatibility

The coordinator `play_voice` path remains separate from media transmission.
It still replaces music, preserves the music ring for resume, reports audible
completion, supports scheduled start and can be stopped by existing player
commands. It uses the render-safe voice snapshot/dispatcher but does not enter
the media clip generation state machine or advertise media delivery features.

Real-device timing, duck quality, output-route behavior and audible evidence
remain in the manual hardware-testing epic; this checkpoint claims deterministic
code, unit/race coverage and hosted build evidence only.

## Windows overlay branch

The Windows overlay capability uses this exact pre-master order:

```text
limiter(main_program * duck_gain + overlay * overlay_gain + cues) * master_gain
```

The coordinator freezes the phase-one defaults at `-12 dB`, `250 ms` attack,
`600 ms` release and a `-1 dBFS` local limiter ceiling. The engine begins the
duck envelope 250 ms before the scheduled first clip sample. It consumes the
main ring during pre-duck, clip playback, cancellation and release, including
when the ring returns silence. Missing main audio therefore does not attenuate
the clip.

`fade_stop` applies its wire `fade_ms` to the overlay branch while the duck
branch returns over `release_ms`; cancellation is acknowledged only when both
ramps are terminal. Natural completion reports the last clip sample immediately
and keeps the render-owned release tail until main gain reaches one. Sanitized
telemetry exposes only aggregate overlay frame, limiter-hit, ring-fill and
underrun counts—never PCM, media identity, paths or protocol payloads.

## macOS overlay branch

The macOS engine uses the same frozen pre-master order as Windows:

```text
DynamicsProcessor(main_source * duck_gain + overlay * overlay_gain + legacy inserts)
    * final mainMixer gain
```

Preparation reads and decodes the whole clip away from the render callback,
then converts it to the engine's 44.1 kHz stereo format. Arming only publishes
the prepared buffer to a dedicated `AVAudioPlayerNode` at an absolute host
time. A serial control queue schedules the T-250 pre-duck and catches a late
control callback up to the envelope instead of restarting the attack at T.
The defaults remain `-12 dB`, `250 ms` attack and `600 ms` release.

The source node continues one unconditional ring read per render callback in
every overlay state. Overlay scheduling, fade timers, decoding, completion
dispatch and telemetry remain outside that callback. The Apple dynamics unit
is configured with a `-1.1 dB` threshold and its minimum `0.1 dB` headroom,
which freezes the final pre-master ceiling at `-1 dBFS`; the user's local
master gain is applied after it.

Natural completion releases the duck and immediately makes the graph available
for a prepared successor. Cancellation applies a raised-cosine overlay fade,
releases the music branch concurrently and acknowledges only after both active
ramps finish. It then stops/resets the overlay node and restores unity gain.
Telemetry is limited to aggregate overlay frames, limiter-hit windows, ring
fill and underruns, matching the Windows privacy boundary.
