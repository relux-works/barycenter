## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:17:33Z

## Last Update
2026-07-15T23:45:48Z

## Blocked By
- TASK-260712-2rlkp7
- TASK-260712-1x0lot

## Blocks
- TASK-260712-2zoy4u
- TASK-260712-wt2n7m
- TASK-260712-2vipy3
- TASK-260712-1vklop
- TASK-260712-285pag

## Checklist
- [x] Persist actor, policy version and hash, locale and acceptance time without content metadata
- [x] Re-prompt on material policy changes and show equivalent RU and EN rights text

## Notes
2026-07-16 strict-sequence start from synchronized main f52fae1 after acceptance of TASK-260712-2bk0vy. Implementing inline outside task-board spawn workflow. Freezing server-owned version/hash and localized RU/EN rights text, recording ActorContext acceptance without content metadata, and gating only the contract-required user-media/transmission/replay paths with deterministic 428 behavior. No claim that consent proves rights or legal validity.
Accepted on engineering heads 0ff2a72 and ba19727. The coordinator now owns immutable content-policy version/hash/locale grants with server acceptance time, actor/orbit scope, revocation and mutation audit/rate limits while storing no content names, filenames or raw transport identifiers. Generic app upload and new Telegram audio/document paths require a current grant; microphone and legacy Telegram voice behavior remain frozen. RU/EN policy manifests share approved Terms and Content Guidelines URLs, material hash/revocation returns deterministic 428 and per-upload rights acknowledgement stays distinct from Terms acceptance. macOS and Windows clients/UI enforce the same contract. Local Go, vet, rollback, Swift Xcode and Windows race/cross-build gates passed; hosted run 29459297560 passed all four jobs after the clean-build UI seam test fix. PR #126 merged as c647b2d. This records consent evidence only and does not claim ownership, legal validity, Store acceptance or real-hardware evidence.

## Precondition Resources
- [p2-targets-inbox-parity-components.puml](file://TASK-260712-2ctf3x/p2-targets-inbox-parity-components.puml) — Phase 2 consent and target service boundaries

## Outcome Resources
- [p2-versioned-content-policy-consent.md](file://TASK-260712-2ctf3x/p2-versioned-content-policy-consent.md) — Versioned consent implementation and downstream handoff
- [targets-inbox-contract-v1.json](file://TASK-260712-2ctf3x/targets-inbox-contract-v1.json) — Frozen policy version, hashes, locale copy and gating contract
- [coordinator-acceptance-manifest.json](file://TASK-260712-2ctf3x/coordinator-acceptance-manifest.json) — Local coordinator automated acceptance provenance
- [swift-acceptance-manifest.json](file://TASK-260712-2ctf3x/swift-acceptance-manifest.json) — Clean-build-equivalent Swift automated acceptance provenance
- [windows-acceptance-manifest.json](file://TASK-260712-2ctf3x/windows-acceptance-manifest.json) — Windows race and cross-build acceptance provenance
