## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:39:53Z

## Last Update
2026-07-15T01:44:50Z

## Blocked By
- TASK-260712-2lrpc0
- TASK-260712-1c04pk
- TASK-260712-30abcm

## Blocks
- TASK-260712-1s6h6t
- TASK-260712-2psvhu

## Checklist
- [x] Implement builtin-cue and exactly five-second record-then-play paths through the production clip output
- [x] Prove the self-test performs no coordinator, upload or telemetry call and deletes local drafts
- [x] Implement picker and drag-drop metadata review with limit and rights guidance

## Notes
2026-07-15 strict sequential kickoff from synchronized main merge a5351f4cc02d72b280a67e8e8b206a0baee3417b after PR #54. Implementing the macOS offline self-test and short-file draft intake inline over the accepted builtin-cue, MacMicrophoneCaptureEngine and production clip-output seams. Real microphone, audible playback/output routing, Finder drag/drop and physical UI observations remain manual EPIC-260714-th54l3 evidence.
2026-07-15 engineering head 50b872d: added local-only production mixer facade with mixer telemetry disabled, exact five-second cue/capture/cue/playback sequencing, close/delete cleanup, content-probed 50 MiB/180 s picker and drag-drop review, streaming canonical PCM16 intake, EN/RU UI state and 193-test coverage. Local Go tests, release Swift build and packaged app/cue/plist/codesign checks pass. PR #55 hosted CI is pending; no real hardware claim is made.
2026-07-15 root acceptance: exact engineering head 50b872d passed all four hosted jobs in GitHub Actions run 29382291652. Root diff review confirmed content-signature probing, fail-closed 50 MiB/180 s boundaries, security-scoped recheck, bounded streaming conversion, exact five-second sequence, production mixer reuse with collectTelemetry=false, and synchronous close/delete cleanup. Real microphone, audible route and Finder UI evidence remain only in EPIC-260714-th54l3.

## Precondition Resources
- [p1-main-ui-capture-flows.puml](file://TASK-260712-3lg0ht/p1-main-ui-capture-flows.puml) — Exact offline self-test and send flow

## Outcome Resources
- [p1-macos-local-self-test-components.puml](file://TASK-260712-3lg0ht/p1-macos-local-self-test-components.puml) — Implemented offline self-test, fail-closed file intake, private drafts and production clip-output boundaries
- [p1-macos-local-self-test-file-intake.md](file://TASK-260712-3lg0ht/p1-macos-local-self-test-file-intake.md) — Implementation contract, automated evidence and manual-test boundary
