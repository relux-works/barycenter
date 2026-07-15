## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T19:39:01Z

## Blocked By
- TASK-260712-14u0yk
- TASK-260712-dqdoqj

## Blocks
- TASK-260712-ibuaxj

## Checklist
- [x] Shortlist bundleable decoder candidates or frameworks that could ship on both platforms
- [x] Build reproducible packaged prototypes with range-backed prepare and bounded-memory playback
- [x] Prove MP3 AAC and Opus decode plus scheduled start, pause, seek, and resume
- [x] Capture package-size, signing, notarization, and MSIX redistribution evidence
- [x] Record a clear viable versus rejected outcome with concrete reasons and no unresolved shipping assumptions
- [x] Account for every transitive signed binary, architecture, notice and vulnerability update path

## Notes
Strict inline execution started from synchronized main a39e2a6 after TASK-260712-1vdlkw engineering PR #106 and tracking PR #107 passed hosted CI and merged. Prototyping only the audited minimal shared FFmpeg 8.1.2 shape, with reproducible source/config/import/SBOM receipts, package-local signed binary inventory, range-backed bounded preparation, lifecycle and hostile-input automation. Real notarization, Store acceptance, audible quality and physical-hardware timing remain unclaimed and fail closed.
Accepted on engineering head ad51481, merged by PR #108 as 666220d. Exact FFmpeg 8.1.2 LGPL-only allowlist, six frozen fixtures, narrow bridge and bounded private-cache harness pass locally and on hosted macOS ARM64, macOS Intel and Windows amd64. Hosted run 29444807851 passed 3/3: macOS ARM64 2,100,847 package bytes, 5,210,112 peak RSS and 93 ms total decode CPU; Windows AppContainer MSIX 1,965,989 bytes, SHA-256 c003ceab37a35b21e9bfc8bea168eed735fbf5c0b964355c0c59a860d15bcb50, 7,028,736 peak RSS and 124 ms total decode CPU. Windows package records and signs all PE imports including exact winpthreads 14.0.0.r190.g96fb1bff7-1, installs under temporary machine TrustedPeople trust, decodes offline from an in-package fixture, uninstalls and removes trust. Standard CI run 29444811403 passed 4/4. Shipping remains explicitly rejected pending Windows ARM64, production Partner Center signer, Developer ID/notarization, release SBOM/advisory/counsel review and accepted native hostile-input isolation; no Store, production-signing or physical-hardware claim is made.

## Precondition Resources
(none)

## Outcome Resources
- [bundled-probe-v1.json](file://TASK-260712-1canzv/bundled-probe-v1.json) — Fail-closed exact source, configure, package, platform and shipping-decision contract
- [p2-bundled-signed-decoder-probe.md](file://TASK-260712-1canzv/p2-bundled-signed-decoder-probe.md) — Reviewed engineering decision, containment boundary, reproduction and release gates
- [repository-acceptance-manifest.json](file://TASK-260712-1canzv/repository-acceptance-manifest.json) — Local repository acceptance manifest; all 12 commands passed
- [receipt-macos-arm64.json](file://TASK-260712-1canzv/receipt-macos-arm64.json) — Hosted signed package inventory: 2,100,847 bytes, exact dylib imports and hashes
- [decode-evidence-macos-arm64.json](file://TASK-260712-1canzv/decode-evidence-macos-arm64.json) — Hosted ARM64 6/6 decode, lifecycle, hostile, CPU and RSS evidence
- [receipt-windows-amd64.json](file://TASK-260712-1canzv/receipt-windows-amd64.json) — Hosted test-signed AppContainer MSIX inventory, signer/import hashes and offline installed decode
- [decode-evidence-windows-amd64.json](file://TASK-260712-1canzv/decode-evidence-windows-amd64.json) — Hosted Windows 6/6 decode, lifecycle, hostile, CPU and RSS evidence
- [toolchain-components.txt](file://TASK-260712-1canzv/toolchain-components.txt) — Exact hosted winpthreads runtime package version receipt
