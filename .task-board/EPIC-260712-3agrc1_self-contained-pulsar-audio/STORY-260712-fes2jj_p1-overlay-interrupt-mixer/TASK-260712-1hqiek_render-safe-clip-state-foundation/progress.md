## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:42:17Z

## Last Update
2026-07-14T17:01:49Z

## Blocked By
- TASK-260712-51y5k9
- TASK-260712-1g70av

## Blocks
- TASK-260712-2zbmq4
- TASK-260712-1viwvi
- TASK-260712-8mwyiv
- TASK-260712-1g6lk8
- TASK-260712-3aj8w2
- TASK-260712-17w78q
- TASK-260712-1ckdr7
- TASK-260712-19w1qn

## Checklist
- [x] Replace render-time mutable ownership with preallocated clip or mixer state handoff on both nodes
- [x] Define shared duck, interrupt, limiter and telemetry parameter carriers used by both platforms
- [x] Preserve and document the legacy play_voice compatibility path while the new media lifecycle lands
- [x] Use generation-safe prepared, armed, playing, cancelling and terminal state on both platforms
- [x] Prove render handoff is preallocated and free of I/O, waits, allocation and blocking locks
- [x] Make active sender-delete generation-safe and expose the frozen terminal reason

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge 74b4e04 after TASK-260712-3t9nr8 engineering acceptance. Scope is repository code, unit/race tests, static render-path proof and hosted builds. No real-device audio quality or hardware observation will be claimed; such evidence remains in the manual-test epic. Critical irreversible owner questions, if any, go to EPIC-260714-zmnd4n without stopping safe best-effort implementation.
2026-07-14 engineering checkpoint: exact code head 8521b84 is published in draft PR #35; hosted run 29351870335 is pending. Both clients now use immutable MixerControlParameters and generation-safe prepared/armed/playing/cancelling/terminal ownership. Windows render uses atomic snapshots and a pre-created bounded completion dispatcher; macOS uses a fixed SPSC gain-command queue, atomic telemetry, predecoded PCM and an off-render first-sample dispatcher. Local coordinator vet/tests, Windows vet/full/race/amd64+arm64 cross-builds and Swift release build pass. Local swift test remains unavailable because this CommandLineTools image lacks module Testing; hosted node-core will decide acceptance. No physical-device evidence is claimed.
2026-07-14 accepted: exact engineering code head 8521b84 passed all four hosted jobs in run 29351870335, including authoritative Swift tests, Windows tests/cross-build and signed packaged probe. All six checklist items are satisfied by the shared carrier, symmetric generation lifecycle, preallocated/atomic render handoff, frozen superseding-delete tombstone and documented separate legacy voice path. Real-device/audible evidence remains explicitly unclaimed in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
(none)
