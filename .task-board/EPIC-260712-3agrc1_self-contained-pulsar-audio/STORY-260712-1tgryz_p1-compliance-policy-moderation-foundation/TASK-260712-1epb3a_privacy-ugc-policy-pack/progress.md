## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:45:03Z

## Last Update
2026-07-14T15:37:41Z

## Blocked By
- TASK-260712-g9ycx5
- TASK-260712-16zfvu

## Blocks
- TASK-260712-pbfz37
- TASK-260712-34stvx
- TASK-260712-dlltnr
- TASK-260712-3t9nr8
- TASK-260712-e1ie4x
- TASK-260712-2s4e9p
- TASK-260712-1x0lot

## Checklist
- [x] Map every required disclosure from spec sections 15.1 and 15.2 into named policy sections.
- [x] Draft RU and EN privacy, terms, content guideline and upload rights copy with exact link targets for app, Telegram, site and Store surfaces.
- [x] Confirm backup, retention and deletion language matches the ingest design and does not promise unsupported moderation or autoplay behavior.
- [x] Save a short cross reference table showing which policy artifact is consumed by which product surface.
- [x] Cover accountless credentials, optional integrations, non-E2EE limitation and target-limited sharing
- [x] Record legal review status and trace every factual statement to a shipped control or approved input

## Notes
2026-07-14 kickoff: strict sequential execution started inline from synchronized main d40b754493b78bb58c24b4fc759312c4a0463533. Approved legal/ops defaults in docs/compliance/legal-ops-inputs.json are canonical. Real-app and physical-hardware validation remains in the separate manual-test epic; this task covers policy sources, traceability, automated validation, and best-effort repository verification.
2026-07-14 engineering acceptance: exact code head 27c19cdad6711f6790594a63fa4ec0a51687f062. Authored versioned EN/RU Privacy, Terms, Content Guidelines and upload-rights notice; all 44 stable sections have exact-hash parity and factual traceability to approved inputs, shipped contracts, explicit limitations or current primary sources. Added strict Go validator/CLI and Store submit gate; obsolete collects-no-personal-data draft is retired. Local coordinator vet/full/race, Windows vet/test/cross-build, Swift release, JSON/board/diff checks passed. Hosted CI run 29345880750 passed all four jobs. Exact-content publication remains honestly hold pending Ivan Oparin review; no public URL, real-app or physical-hardware result is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-privacy-ugc-policy-pack.md](file://TASK-260712-1epb3a/p1-privacy-ugc-policy-pack.md) — Bilingual policy source map, surface ownership, traceability, review state and delta triggers
- [policy-pack-2026-07-14.json](file://TASK-260712-1epb3a/policy-pack-2026-07-14.json) — Exact-hash machine contract for EN/RU artifacts and fail-closed publication review
