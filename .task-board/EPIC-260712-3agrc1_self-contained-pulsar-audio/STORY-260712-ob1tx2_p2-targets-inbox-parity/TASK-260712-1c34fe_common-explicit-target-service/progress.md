## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:51Z

## Last Update
2026-07-15T22:30:40Z

## Blocked By
- TASK-260712-2rlkp7
- TASK-260712-2qpp6w
- TASK-260712-2vhf80
- TASK-260712-kr64r2
- TASK-260712-25862f

## Blocks
- TASK-260712-2j5fkr
- TASK-260712-1vklop
- TASK-260712-2vipy3
- TASK-260712-2h6snp

## Checklist
- [x] Resolve own current Air and explicit audiences into exact target snapshots
- [x] Apply include origin and replay semantics without caller owned ordering
- [x] Remove the personal delivery broadcast fallback for more than one recipient
- [x] Expose unsupported target policy instead of silent downgrade
- [x] Authorize opaque target selectors and deduplicate exact node snapshots
- [x] Remove every N-recipient broadcast fallback and enforce explicit mixed-version policy

## Notes
2026-07-15 strict-sequence start from synchronized main 3f636be. Implementing inline outside task-board spawn workflow. Extending the shared Phase 1 transmission service under the frozen p2-targets-inbox-parity.v1 contract: authorized opaque selectors, exact immutable deduplicated node snapshots, include-origin and Air policy, no N-recipient broadcast fallback, explicit unsupported/mixed-version outcomes, and manual replay as a new transmission. Automated unit and integration evidence only; no real-hardware claims.
2026-07-15 strict-sequence implementation completed inline on task/task-260712-1c34fe-common-explicit-target-service. The shared Phase 1 acceptance transaction now accepts only opaque trf_ selectors externally, stores only capability digests, binds references to ActorContext plus credential scope, and revalidates domain and Pulsar binding generation. GET /v1/transmission-targets exposes only authorized references/kinds/labels. Explicit targets are deduplicated before include-origin filtering and fail atomically with 422 unsupported_targets plus opaque sorted capability details; one policy covers Phase 1 clips and future queue/replace tracks. Production Telegram mints the same refs and no longer falls back to legacy routing; rollback-era unbound rows refuse unrepresentable N-recipient personal delivery rather than broadcast. Local coordinator full suite, coordinator acceptance/vet/rollback suite, Windows portable/race/cross-build suite, and Swift 221-test suite all passed. Preparing engineering PR and hosted standard CI; no real-hardware claim.
Accepted after engineering PR #122 merged as 1cc0759 (head 5b69232). Hosted CI run 29455392790 passed coordinator, node-core, pulsar-win and packaged Windows probe. Opaque target references, exact domain/binding reauthorization, deterministic deduplication, include-origin filtering, atomic mixed-version unsupported_targets, common clip/future-track capability policy, production Telegram parity and no personal-to-broadcast fallback are covered by automated tests. Local pinned coordinator, Windows and Swift suites also passed; no physical hardware evidence is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-components.puml](file://TASK-260712-1c34fe/p2-targets-inbox-parity-components.puml) — Implemented common explicit-target application-service context
- [p2-common-explicit-target-service.md](file://TASK-260712-1c34fe/p2-common-explicit-target-service.md) — Opaque target, mixed-version, Telegram and downstream handoff
- [hosted-coordinator-manifest.json](file://TASK-260712-1c34fe/hosted-coordinator-manifest.json) — Hosted CI run 29455392790 coordinator acceptance manifest
- [hosted-swift-manifest.json](file://TASK-260712-1c34fe/hosted-swift-manifest.json) — Hosted CI run 29455392790 Swift acceptance manifest
- [hosted-windows-manifest.json](file://TASK-260712-1c34fe/hosted-windows-manifest.json) — Hosted CI run 29455392790 Windows acceptance manifest
