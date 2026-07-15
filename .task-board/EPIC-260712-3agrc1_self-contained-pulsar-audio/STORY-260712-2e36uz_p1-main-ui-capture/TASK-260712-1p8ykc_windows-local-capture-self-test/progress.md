## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-15T03:45:09Z

## Blocked By
- TASK-260712-2lrpc0
- TASK-260712-9i5se7
- TASK-260712-2w4gyw
- TASK-260712-25at8b
- TASK-260712-c7dmv8

## Blocks
- TASK-260712-2fe5bz

## Checklist
- [x] Implement permission-aware input and output selection, level metering and the local loopback path.
- [x] Wire toggle recording, start and stop cues, Esc cancel, temp-file cleanup and clean stop on quit, lock and permission revoke.
- [x] Integrate RegisterHotKey, the standard file picker and drag-drop with clear fallback behavior when the hotkey is unavailable.

## Notes
2026-07-15 kickoff: strict inline execution started from synchronized main c0f0509f55fb2162ea67a42ee62f0925c416b55b after PR #59. Scope is deterministic integration of the accepted Windows capture, self-test/file intake, shell and tray-hotkey seams; real AppContainer permission UI, microphone/output, audible cues, Explorer picker/drop, physical shortcut, Narrator, lock/suspend and packaged-app observations remain manual in EPIC-260714-th54l3.
2026-07-15 engineering acceptance: d29f391 integrates paired and accountless local capture through one workflow owner; stable input and live output selection, meter, production loopback, cues, five-second self-test, durable normal draft, FileOpenPicker, Explorer drop, window/tray/RegisterHotKey parity, Esc and typed lifecycle cleanup are covered. Gates passed: pulsar-win go test, go test -race, go vet, Windows amd64 cross-vet/build; coordinator go vet/test; node-app 205 Swift tests. No real hardware/AppContainer/audible observation is claimed; manual evidence remains EPIC-260714-th54l3.
Hosted acceptance: PR #60 engineering head d29f391 passed all four jobs in run 29387172394, including native helper tests and reproducible signed MSIX packaging without claiming hardware evidence.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-1p8ykc/p1-main-ui-capture-components.puml) — Accepted component diagram for the integrated Windows local capture workflow
- [p1-main-ui-capture-flows.puml](file://TASK-260712-1p8ykc/p1-main-ui-capture-flows.puml) — Flow diagram for local self-test and record/send behavior
- [p1-windows-local-capture-self-test-integration.md](file://TASK-260712-1p8ykc/p1-windows-local-capture-self-test-integration.md) — Engineering handoff and automated/manual evidence boundary
