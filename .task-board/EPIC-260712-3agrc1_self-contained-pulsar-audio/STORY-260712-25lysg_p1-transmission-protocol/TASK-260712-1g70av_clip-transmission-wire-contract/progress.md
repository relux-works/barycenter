## Status
to-review

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T08:28:14Z

## Blocked By
- TASK-260712-51y5k9

## Blocks
- TASK-260712-31vvjt
- TASK-260712-2bbz13
- TASK-260712-26ip33
- TASK-260712-2qc27p
- TASK-260712-1hqiek
- TASK-260712-31rkpe

## Checklist
- [x] Add Go protocol messages and capability flags for clip transmissions
- [x] Update golden JSON plus Go codec tests and protocol documentation
- [x] Mirror the contract in Windows and Swift tests without breaking legacy voice

## Notes
Strict inline execution started from clean synchronized main merge 35d9974e6a2212b6757e6d053d8b896a652ec4f7 on branch task/task-260712-1g70av-clip-transmission-wire-contract. Scope is the frozen p1-transmission-v1 WebSocket codec and capability contract across canonical Go, Windows, and Swift mirrors while preserving legacy play_voice and solo_voice. Manual real-app and hardware validation remains deferred to EPIC-260714-th54l3.
Implemented 10 additive clip, cancellation, DND and presence wire messages in the canonical Go codec and byte-pinned Windows mirror, plus the Swift Message mirror and 39 shared golden fixtures. Added immutable validated capability snapshots, exact reconnect replacement in the coordinator loop, malformed registration rejection, legacy voice compatibility and conditional-field tests. Root review also corrected all malformed legacy golden msg_ and el_ identifiers and added recursive guards. Full coordinator and Windows vet/test, focused race gates, Windows cross-build, Swift build, JSON, mirror, diff and board validation pass. Local Swift tests remain limited only by the known no such module Testing workstation toolchain gap; hosted macOS CI is required before acceptance. Outcome: docs/analysis/p1-clip-transmission-wire-contract.md. No manual app or hardware evidence is claimed.

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-1g70av/p1-transmission-scheduler-sequence.puml) — Wire contract flow for prepare barrier and legacy downgrade

## Outcome Resources
- [TASK-260712-1g70av_clip-transmission-wire-contract.md](file://TASK-260712-1g70av/TASK-260712-1g70av_clip-transmission-wire-contract.md) — Implemented additive clip, DND, presence and capability wire contract with automated compatibility evidence
