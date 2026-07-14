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
duck `0...1`, non-negative fades, limiter ceiling `-1 dBFS`. The prepared media
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
