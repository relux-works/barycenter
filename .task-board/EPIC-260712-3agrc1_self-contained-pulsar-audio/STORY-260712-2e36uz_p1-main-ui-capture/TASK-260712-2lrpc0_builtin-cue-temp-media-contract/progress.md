## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:29:05Z

## Last Update
2026-07-15T00:06:40Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1p8ykc
- TASK-260712-1s6h6t
- TASK-260712-25at8b
- TASK-260712-3lg0ht

## Checklist
- [x] Select or create the redistributable cue asset and record provenance.
- [x] Define the shared temporary-media lifecycle and no-upload guarantees for self-test and recording drafts.
- [x] Publish the asset path, loading contract and failure copy for both platform tasks.
- [x] Separate disposable self-test, active partial capture and durable unsent user-draft states
- [x] Sequence start and stop cues outside committed microphone samples and recover crashes safely

## Notes
2026-07-15 strict sequential kickoff from synchronized main 0008147512fdd4d82d9acc7b40a6a61174e490f8 after PR #50. Freezing the shared cue provenance, package/load contract and crash-safe disposable partial durable-draft lifecycle inline with deterministic code and unit tests; no microphone, audible, real-app, packaged-device or physical-hardware result will be claimed.
2026-07-15 acceptance: exact engineering head 52fa9ea40bb400ee5e5e7ca77eb0769eea04f9fc passed all four hosted jobs in CI run 29377961960. Added deterministic Relux Works-owned PCM cue with exact provenance and package guards; shared Swift/Go 17-transition lifecycle; owner-only opaque partial/self-test/durable draft stores; fsync plus atomic rename; picker-copy/token closure; path-free errors; restart recovery; cue sequencing; tests, docs and rendered diagrams. Hosted macOS ran 180 Swift tests; signed MSIX package and install-contract job passed. No real microphone, audible cue, real-app or physical-hardware evidence is claimed; those remain in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-2lrpc0/p1-main-ui-capture-components.puml) — Implemented cross-platform cue, lifecycle, storage, package, and recovery architecture
- [p1-main-ui-capture-flows.puml](file://TASK-260712-2lrpc0/p1-main-ui-capture-flows.puml) — Frozen recording, picker intake, upload retention, and startup recovery flows
