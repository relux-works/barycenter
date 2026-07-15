## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T18:00:20Z

## Blocked By
- TASK-260712-14u0yk

## Blocks
- TASK-260712-ibuaxj
- TASK-260712-2eympi
- TASK-260712-2g3fkt

## Checklist
- [x] Collect the exact source, version, and package shape for every candidate library or framework
- [x] Record redistribution, notice, and patent or commercial-license obligations for each candidate
- [x] Assess Microsoft Store, AppContainer, codesign, and notarization implications
- [x] Reject any path that depends on first-run downloads or undocumented sandbox weakening
- [x] Publish a reviewed shippable versus rejected matrix that the final ADR can cite directly
- [x] Record exact versions, transitive licenses, SBOM, CVE ownership and authoritative retrieval dates

## Notes
Strict inline execution started from synchronized main b66c38f after TASK-260712-dqdoqj engineering PR #104 and tracking PR #105 passed hosted CI and merged. Auditing exact candidate licenses, patent/codec posture, redistribution, dynamic/static linking, source/notice obligations, AppContainer/MSIX and macOS signing/notarization constraints from primary sources; legal counsel conclusion remains explicitly unclaimed.
Accepted on exact engineering code head 3fc2409. Audited seven exact components and all three frozen candidates from primary sources dated 2026-07-15. pure-go-composite-v1 is rejected because go-aac is GPL-2.0-only, FAAD2-derived and its origin was unavailable; native-canonical-aac-v1 and bundled-ffmpeg-8.1.2-v1 are shippable-with-obligations only, with AAC counsel approval, exact notices/source/SBOM/CVE gates, immutable signed package members, sandbox retention and no runtime code download. Fail-closed validator and tamper test pass; codec suite 9/9 and exact local repository acceptance 12/12. Hosted run 29437923424 attempt 1 was cancelled after a packaged-runner infrastructure hang; clean rerun passed pulsar-win 1m47s, node-core 1m47s, coordinator 2m19s and packaged probe 2m41s. PR #106 merged as 594495b. No legal advice, patent clearance, decoder correctness, Store acceptance, audible or hardware result is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [license-audit-v1.json](file://TASK-260712-1vdlkw/license-audit-v1.json) — Fail-closed exact component, license, supply-chain and release-gate disposition
- [p2-codec-license-distribution-audit.md](file://TASK-260712-1vdlkw/p2-codec-license-distribution-audit.md) — Reviewed source-cited candidate matrix and downstream spike handoff
- [THIRD_PARTY_NOTICES.codec-spike.txt](file://TASK-260712-1vdlkw/THIRD_PARTY_NOTICES.codec-spike.txt) — Audited notice and release template with rejected-component tripwire
- [repository-acceptance-manifest.json](file://TASK-260712-1vdlkw/repository-acceptance-manifest.json) — Exact local repository acceptance manifest; 12 of 12 commands passed
