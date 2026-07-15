## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:39:53Z

## Last Update
2026-07-15T02:29:39Z

## Blocked By
- TASK-260712-2lrpc0
- TASK-260712-9i5se7
- TASK-260712-2w4gyw

## Blocks
- TASK-260712-1p8ykc
- TASK-260712-3lximx

## Checklist
- [x] Implement builtin-cue and exactly five-second record-then-play paths through the production clip output
- [x] Prove the self-test performs no coordinator, upload or telemetry call and deletes local drafts
- [x] Implement brokered picker and drag-drop metadata review with limit and rights guidance

## Notes
2026-07-15 strict sequential kickoff from synchronized main merge 893125faa25744b08148ea6f72b364e3c823bb77 after PR #56. Implementing the Windows offline builtin-cue and exact five-second record-then-play path plus brokered short-file review inline over the accepted shell, capture and media lifecycle. No real microphone, audible route, Explorer picker/drop or physical-device result will be claimed; those remain manual EPIC-260714-th54l3 evidence.
2026-07-15 implementation outcome: added a direct local-only facade over the production Windows overlay mixer with report telemetry disabled and no MediaClipClient, fetch, coordinator or upload ownership; added exact five-second self-test orchestration and self_test capture classification; added generation-safe cancel, close, replacement and failure cleanup; added strict broker-authorized stream review and bounded canonical PCM16 private intake with honest local decoder limitations; staged the reviewed cue in the production MSIX. Local coordinator tests, 202 Swift tests, Go vet, full Windows race suite, 82.6 percent new-flow coverage, Windows amd64 cross-build and YAML/board validation passed. GitHub Actions run 29384112933 passed all four jobs including hosted Windows tests and signed MSIX package/install/cleanup. Real microphone, audible output, permission UI, Explorer picker/drop, clean install and AppContainer observations remain manual EPIC-260714-th54l3 evidence. Accepted engineering head 88868cc64f6e7e1059cf7fe759eade71f097cf92.

## Precondition Resources
- [p1-main-ui-capture-flows.puml](file://TASK-260712-25at8b/p1-main-ui-capture-flows.puml) — Exact offline self-test and send flow

## Outcome Resources
- [p1-windows-local-self-test-file-intake.md](file://TASK-260712-25at8b/p1-windows-local-self-test-file-intake.md) — Offline production-output flow, strict brokered intake, format limits and manual boundary
- [p1-windows-local-self-test-components.puml](file://TASK-260712-25at8b/p1-windows-local-self-test-components.puml) — Implemented Windows self-test, production mixer and brokered file component boundaries
