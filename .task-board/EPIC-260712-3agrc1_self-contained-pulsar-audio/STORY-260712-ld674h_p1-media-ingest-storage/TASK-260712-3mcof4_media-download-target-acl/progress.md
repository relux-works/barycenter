## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:40:50Z

## Last Update
2026-07-14T04:22:56Z

## Blocked By
- TASK-260712-z6h6wh
- TASK-260712-2af2dp
- TASK-260712-1bpog0

## Blocks
- TASK-260712-gj0cko
- TASK-260712-1aprcb
- TASK-260712-3lf8r0

## Checklist
- [ ] Authorize owner and immutable transmission-target snapshot reads with separated node and control capabilities
- [ ] Return uniform non-disclosing failures for foreign, revoked, deleted and expired access
- [ ] Add negative tests for guessed IDs, copied URLs and membership changes after transmission acceptance

## Notes
Strict sequential inline execution started 2026-07-14 from clean main merge fe8e73c6c8d7dd2f05a3ff0acc4926ef30afa169 on branch task/task-260712-3mcof4-media-download-target-acl. The target-snapshot ACL is implemented as an independently testable seam and will be connected to transmission persistence only by its downstream owner; no live-membership shortcut, real-app result or hardware evidence is claimed.
Implementation checkpoint: generic owner control GET and fail-closed immutable target-snapshot reader are implemented; node/control domains are separated; live credential/media authorization is repeated and held through descriptor acquisition; canonical symlinks are refused; legacy node-only approach compatibility remains isolated. Focused race tests, full coordinator vet/test/race, exact previous-head rollback suite, pulsar-win vet/test/Windows build and board validation pass. Local node-app swift test remains blocked by the pre-existing missing Testing module in this workstation toolchain; no manual real-app or hardware evidence is claimed.

## Precondition Resources
- [p1-media-ingest-component.puml](file://TASK-260712-3mcof4/p1-media-ingest-component.puml) — Media authorization and target ACL boundary

## Outcome Resources
- [TASK-260712-3mcof4_p1-media-download-target-acl-contract.md](file://TASK-260712-3mcof4/TASK-260712-3mcof4_p1-media-download-target-acl-contract.md) — Generic media owner/immutable-target ACL, live revocation, canonical byte and legacy compatibility contract
