## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:27:54Z

## Last Update
2026-07-13T23:23:26Z

## Blocked By
- TASK-260712-dib11l
- TASK-260712-2y74io

## Blocks
- TASK-260712-1vtwkl
- TASK-260712-298tyq

## Checklist
- [x] Add only the manifest declarations required by the selected probe APIs
- [x] Document the signing path used for local or Store-distributed hardware proof
- [x] Write install and run instructions that name where logs and artifacts land

## Notes
2026-07-14 strict sequential inline execution started from clean landed main 182c203 on branch task/task-260712-13rbnw-signed-msix. No task-board spawn workflow; first pass audits AC, existing package/sign scripts, artifacts, toolchain availability, and reproducible offline verification boundaries.
2026-07-14 implementation draft freezes current Partner Center identity/Publisher, exact four-capability AppContainer contract, certificate-store SHA-256 signing, safe self-signed trust/install/launch receipt, and an unsigned Store-flight route. Local pulsar-win go test ./... and diff/YAML checks pass; hosted Windows MakeAppx/SignTool/install CI is pending.
2026-07-14 implementation complete at f5b73f0. Final CI 29292631211 is green, including frozen negative contracts, native CTest, MakeAppx, SHA-256 SignTool, safe manifest preflight, trusted registration, digest-bound receipt and artifact upload. Signed MSIX SHA-256 a0c3022b69c68f140969a7d7bef4cd0904f1b2872960e7a4511bea9462749be7; real Win10/Win11 hardware and WACK remain explicitly downstream.
2026-07-14 frozen independent review and root line-by-line audit PASS on production commit f5b73f0 and final run 29292631211. Closed findings cover native PACKAGE_ID layout, PowerShell EKU adaptation, pre-trust contract validation, artifact validity, bounded launch, DTD/size limits, byte-digest binding and failure cleanup. No unresolved task-scope finding remains.

## Precondition Resources
- [p1-windows-store-spike-components.puml](file://TASK-260712-13rbnw/p1-windows-store-spike-components.puml) — Component view for manifest and packaging work
- [TASK-260712-13rbnw_implementation-guard.md](file://TASK-260712-13rbnw/TASK-260712-13rbnw_implementation-guard.md) — Mandatory signed MSIX packaging and evidence guardrails

## Outcome Resources
- [TASK-260712-13rbnw_implementation-outcome.md](file://TASK-260712-13rbnw/TASK-260712-13rbnw_implementation-outcome.md) — Signed MSIX implementation, acceptance mapping, exact hashes, CI artifact and residual hardware gates
- [TASK-260712-13rbnw_independent-review-r1.md](file://TASK-260712-13rbnw/TASK-260712-13rbnw_independent-review-r1.md) — Frozen same-executor independent review, closed findings and PASS verdict
- [TASK-260712-13rbnw_root-audit-r1.md](file://TASK-260712-13rbnw/TASK-260712-13rbnw_root-audit-r1.md) — Root line-by-line audit of exact production bytes, artifact and gate truthfulness
