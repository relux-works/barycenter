## Status
closed

## Assigned To
(none)

## Created
2026-07-12T15:44:15Z

## Last Update
2026-07-21T10:59:19Z

## Blocked By
- TASK-260712-3d6cnn
- TASK-260712-2qc27p
- TASK-260712-1vtwkl

## Blocks
- TASK-260712-e5mfqj

## Checklist
- [ ] Run and record overlay continuity on real Windows and macOS outputs with supporting telemetry
- [ ] Run and record interrupt resume on real Windows and macOS outputs with preserved-position measurements
- [ ] Save reviewer-facing evidence and repeatable steps for root review and store-acceptance coordination
- [ ] Keep Spotify-enabled A3/A4 fixtures internal and separately prove standalone clip playback
- [ ] Record sanitized build, hardware, timing and failure evidence for root review
- [ ] Measure same-T ready-target start skew p95 <= 100 ms on exact real Windows/macOS builds and prove stale, offline, and DND work remains inaudible

## Notes
2026-07-14 scope routing: moved to EPIC-260714-th54l3 Manual real-app hardware testing. This task is deferred for hands-on execution in original sequence and no longer gates best-effort coding, unit tests or deterministic CI in EPIC-260712-3agrc1.
2026-07-14 transmission-regression handoff: deterministic TASK-260712-2qc27p proves the three-second barrier, one coordinator T, max-RTT formula, non-autoplay state transitions and timer cleanup. This manual card must additionally observe same-T ready-target p95 start skew <=100 ms, audible non-overlap and inaudible stale/offline/DND work on provenance-tracked real Windows/macOS applications. None of that evidence has been run or claimed yet.
2026-07-21 owner-directed consolidation: closed as superseded, not passed. Remaining highest-value real-app and hardware observations now live only in TASK-260721-ryk8c0 Ivan Oparin final real-app verification, gated by TASK-260721-2346wf Desktop UI automated acceptance and owner handoff.

## Precondition Resources
(none)

## Outcome Resources
(none)
