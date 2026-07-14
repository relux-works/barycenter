## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:57:28Z

## Last Update
2026-07-14T15:10:57Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1epb3a
- TASK-260712-e1ie4x
- TASK-260712-2s4e9p
- TASK-260712-1xik11

## Checklist
- [x] Snapshot official Store policy version, effective date, retrieval date and URLs
- [x] Map 10.1, 10.3, 10.5, 10.6, 10.7, 11.11 and 11.12 plus current asset rules to Pulsar evidence
- [x] Reverify official policies immediately before external submission and record deltas

## Notes
Strict inline execution started from synchronized main c6f1afdb1040bc654b18e324f71b71fd524ca1e7 after PR #30. Produce a dated, official-source-only Microsoft Store and IARC requirements matrix, map every mandatory rule and July 2026 finding to owned implementation or evidence, and retain pre-submit delta verification as an explicit later gate. No task-board spawn workflow and no manual hardware claim.
Accepted engineering code head f0bcacef669dc0c8cfeec694d1e5d0323abbef83. Official Microsoft/IARC retrieval on 2026-07-14 confirms Store Policies v7.19, published 2025-09-10 and effective 2025-10-14. The dated human/machine matrices distinguish mandatory rules from recommendations, map 10.1/10.3/10.5/10.6/10.7/11.11/11.12 and asset/WACK/IARC requirements to concrete owners/evidence, explain the accountless reviewer path and 10.3.2 server duty, and specify six locale-specific corrective screenshots. July finding dates/raw evidence are not overclaimed; real-app capture stays in manual TASK-260712-e5mfqj. A strict Go validator and store-submit workflow require a fresh tag-bound <=24h proceed record and task IDs for every delta; the checked-in initial record is deliberately hold. Local coordinator vet/full/race, Windows vet/test/cross-build, Swift release, board/JSON/diff gates passed. Hosted run 29343948310 passed coordinator, node-core, pulsar-win and signed packaged probe on the exact head.

## Precondition Resources
- [store-policy-baseline-2026-07-12.md](file://TASK-260712-g9ycx5/store-policy-baseline-2026-07-12.md) — Dated official Store policy and certification-finding snapshot

## Outcome Resources
- [store-policy-baseline-2026-07-14.md](file://TASK-260712-g9ycx5/store-policy-baseline-2026-07-14.md) — Dated official-source requirements matrix and corrective evidence map
- [store-policy-baseline-2026-07-14.json](file://TASK-260712-g9ycx5/store-policy-baseline-2026-07-14.json) — Machine-readable Store policy source and task ownership contract
