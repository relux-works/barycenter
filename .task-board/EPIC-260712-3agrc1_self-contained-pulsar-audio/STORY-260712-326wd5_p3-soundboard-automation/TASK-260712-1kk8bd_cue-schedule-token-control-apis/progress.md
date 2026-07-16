## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:37:28Z

## Last Update
2026-07-16T23:11:19Z

## Blocked By
- TASK-260712-3sj8ox
- TASK-260712-3sv87k
- STORY-260712-2ve1c8
- TASK-260712-hb5xz2

## Blocks
- TASK-260712-1eva0y
- TASK-260712-11e4e3
- TASK-260712-288j4a
- TASK-260712-1yw7fo
- TASK-260712-89fzlc
- TASK-260712-1oodka
- TASK-260712-uht9e2

## Checklist
- [x] Add authenticated cue CRUD and listing flows for builtin and user cue entries.
- [x] Add schedule CRUD with timezone, quiet-hour, and target validation.
- [x] Add scoped-principal issue and revoke flows with least-privilege scope enforcement.
- [x] Cover idempotency, stable validation errors, and no-secret logging for the control-plane APIs.

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge 146809abebeb137a1daeb88ba10cd0e9bb5ce18d after TASK-260712-3sv87k code PR #207 and tracking PR #208; hosted runs 29538458103 and 29538727450 passed 4/4. Execute inline outside task-board spawn. Freeze bounded authenticated control-plane scope before editing; runtime scheduler/ratelimits and client composition remain downstream.
2026-07-17 engineering complete. Code head 5722332eae133888e8328fa8e549184357137475; clean exact-head acceptance passed 12/12 with manualEvidence not-run. PR #209 hosted run 29541407173 passed 4/4 and merged as 59fa34dde5ae6a515e786b15bce5a468380d46ed. Same-orbit cue, feature, schedule and scoped-principal controls are accepted with route-bound durable idempotency, canonical target scopes, one-time unrecoverable secrets, immediate revoke and a production-dark trigger seam. Runtime admission, rate limits, scheduling and dispatch remain TASK-260712-1eva0y; no real-app or hardware result is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [acceptance-5722332-manifest.json](file://TASK-260712-1kk8bd/acceptance-5722332-manifest.json) — Exact code-head automated acceptance: 12/12 pass, clean start/end, manualEvidence not-run.
