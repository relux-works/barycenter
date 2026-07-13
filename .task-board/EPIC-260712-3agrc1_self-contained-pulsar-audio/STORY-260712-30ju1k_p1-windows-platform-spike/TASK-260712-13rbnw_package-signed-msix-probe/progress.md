## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:27:54Z

## Last Update
2026-07-13T23:06:34Z

## Blocked By
- TASK-260712-dib11l
- TASK-260712-2y74io

## Blocks
- TASK-260712-1vtwkl
- TASK-260712-298tyq

## Checklist
- [ ] Add only the manifest declarations required by the selected probe APIs
- [ ] Document the signing path used for local or Store-distributed hardware proof
- [ ] Write install and run instructions that name where logs and artifacts land

## Notes
2026-07-14 strict sequential inline execution started from clean landed main 182c203 on branch task/task-260712-13rbnw-signed-msix. No task-board spawn workflow; first pass audits AC, existing package/sign scripts, artifacts, toolchain availability, and reproducible offline verification boundaries.
2026-07-14 implementation draft freezes current Partner Center identity/Publisher, exact four-capability AppContainer contract, certificate-store SHA-256 signing, safe self-signed trust/install/launch receipt, and an unsigned Store-flight route. Local pulsar-win go test ./... and diff/YAML checks pass; hosted Windows MakeAppx/SignTool/install CI is pending.

## Precondition Resources
- [p1-windows-store-spike-components.puml](file://TASK-260712-13rbnw/p1-windows-store-spike-components.puml) — Component view for manifest and packaging work
- [TASK-260712-13rbnw_implementation-guard.md](file://TASK-260712-13rbnw/TASK-260712-13rbnw_implementation-guard.md) — Mandatory signed MSIX packaging and evidence guardrails

## Outcome Resources
(none)
