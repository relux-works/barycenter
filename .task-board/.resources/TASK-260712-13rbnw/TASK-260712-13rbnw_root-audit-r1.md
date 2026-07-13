# TASK-260712-13rbnw — root line-by-line audit R1

Date: 2026-07-14  
Role: root same-executor acceptance audit  
Frozen production commit: `f5b73f06a9e06f71c6193d982e6138e5bec68247`

## Audit boundary

The audit reread the full task card and implementation guard, all changed files rather than diff excerpts, the exact manifest inside the downloaded MSIX, build/install JSON, artifact inventory, CI logs, local test output, working-tree status, and source hashes recorded in the outcome. Historical failed runs were treated as findings, not evidence; only the final run may support acceptance.

## Line review

- Identity: manifest, Go constants, PowerShell constants, derived PFN, installed PFN, and documented AUMID are mutually consistent.
- Sandbox: one x64 Application, `appContainer`, `packagedClassicApp`, no extensions, and exactly the accepted three network plus microphone declarations.
- Signing: exact Publisher/Subject comparison, Code Signing EKU, validity check, SHA-256 SignTool input, embedded signer check, and no PFX/private export.
- Preflight/trust: bounded DTD-free archive parsing precedes the opt-in LocalMachine Trusted People mutation; Authenticode and byte digest are revalidated before install.
- Install/launch: existing production-family installs fail closed; post-install manifest, Publisher, version, architecture, and PFN are checked; AUMID and package-private output paths are derived, not guessed; failure cleans package and newly added trust.
- CI: negative contract tests, native/Go build and tests, signing, registration, digest binding, receipt, and artifact upload are present. No hosted job is described as Windows 10/11 hardware evidence.
- Documentation: local signing, Store signing, install, launch, evidence layout, cleanup, identity collision warning, and downstream gates are explicit.

## Acceptance matrix

| Gate | Result |
| --- | --- |
| Task card and three checklist items | PASS |
| Current Partner Center identity/PFN/AUMID | PASS |
| AppContainer and least-capability posture | PASS |
| Reproducible local signed path | PASS |
| Explicit Store route without unauthorized submission | PASS |
| Signed package build and hosted registration | PASS |
| No secret/private certificate material | PASS |
| Honest external-gate separation | PASS |
| Real Windows 10/11 hardware and WACK | DOWNSTREAM, not claimed |

Final exact-hash CI run `29292631211` is green. The downloaded MSIX SHA-256 is `a0c3022b69c68f140969a7d7bef4cd0904f1b2872960e7a4511bea9462749be7`, and build metadata plus install receipt contain the same digest.

Final verdict: **PASS**. `TASK-260712-13rbnw` may be accepted and landed; only `TASK-260712-1vtwkl` may begin next.
