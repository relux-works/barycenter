## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:40:50Z

## Last Update
2026-07-15T00:23:23Z

## Blocked By
- (none)

## Blocks
- TASK-260712-3lg0ht
- TASK-260712-ut6akw
- TASK-260712-1s6h6t
- TASK-260712-26mnp1

## Checklist
- [ ] Implement explicit TCC-gated capture with default and selected device support
- [ ] Expose bounded metering, hard limits, app-private drafts and main-program ducking
- [ ] Cover cancel, device loss, permission revoke, sleep, lock where observable and quit cleanup
- [ ] Finalize exactly one durable draft and prove cue audio is excluded

## Notes
2026-07-15 strict sequential kickoff from synchronized main merge 1a7d68cd9e8ef2b2ff4b1809bda43757f5c97774 after PR #51. Implementing the macOS capture engine inline against the accepted cue/media lifecycle with explicit TCC-on-record, bounded local PCM, device/meter state, clean terminal paths and no network ownership. Real microphone, audible, sleep/lock/device-loss and physical-hardware observations remain manual EPIC-260714-th54l3 evidence.
2026-07-15 implementation outcome: added explicit Record-only TCC authorization, default/selected CoreAudio input capture through AVAudioEngine, mono 48 kHz PCM16 WAV drafts, local RMS metering, 180-second/50-MiB hard limits, cue-exclusion gate, -12 dB main-program ducking and serialized cleanup for cancel/device/TCC/sleep/session/quit/backend terminals. Deterministic tests include late-TCC-grant cancellation. No real hardware result is claimed; manual evidence remains EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-30abcm/p1-main-ui-capture-components.puml) — Implemented macOS capture lifecycle, TCC, device, cue, duck, meter and private-draft boundaries
