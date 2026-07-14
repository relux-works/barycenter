## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:57:28Z

## Last Update
2026-07-14T16:16:08Z

## Blocked By
- TASK-260712-1epb3a
- TASK-260712-16zfvu

## Blocks
- TASK-260712-pbfz37
- TASK-260712-34stvx
- TASK-260712-dlltnr
- TASK-260712-3t9nr8
- TASK-260712-2s4e9p
- TASK-260712-1xik11
- TASK-260712-38lssj
- TASK-260712-2ctf3x

## Checklist
- [ ] Publish versioned RU and EN policy and support pages over stable unauthenticated HTTPS
- [ ] Wire and automatically check Windows, macOS, Telegram, website and Store links
- [ ] Verify deployed page hashes and rollback or cache behavior before certification

## Notes
2026-07-14 kickoff: strict sequential execution started inline from synchronized main f1048c280aa7bdf6bfd92c7b2a971fc9dc027983. Repository implementation, locale rendering, link checks and deployment/rollback controls may proceed. Live publication and a proceed decision remain gated on Ivan Oparin approving the exact EN/RU hashes from TASK-260712-1epb3a; defaults approval is not recorded as exact-content approval. Real-app/manual link observation remains in the manual-test boundary where applicable.
2026-07-14 staging checkpoint: deterministic generator builds five EN/RU documents into 10 stable and 10 immutable versioned HTML routes with source/rendered hashes, locale switches, stable anchors, cache headers and a public deployment manifest. macOS, Windows, Telegram and Store source links are wired and unit-checked. Local coordinator full/vet/race, Windows vet/race/cross-build, Swift release build, JSON/diff and deterministic regeneration pass. Source pack remains hold and no live publication is claimed until Ivan Oparin approves the exact 10 source hashes.
Staging publication coordinates: Barycenter exact source/generator head 43c0bd992e25c1e85aba6b7a086a94dad378eb35 is in draft PR #33. Generated pulsar-site head 1316a268ac025570a62f9d86a83e56146b5e3779 is in draft PR #1 and pins that upstream commit. Cloudflare preview success is not production publication or acceptance. The site exact-upstream job intentionally requires proceed and is expected to remain closed until exact-hash approval.
Owner gate cleared 2026-07-14T20:09:26+04:00: Ivan Oparin explicitly approved the exact EN/RU source hashes from immutable commit 43c0bd992e25c1e85aba6b7a086a94dad378eb35 for production publication. policy-pack-check --require-proceed now passes; task remains development only until production deployment and live hash/cache verification.
Production probe after pulsar-site merge adb6caffc463a5288bd26ab449acb21f63a92205 caught two custom-domain edge defects: extensionless legal paths returned an empty 200 and Cloudflare Email Address Obfuscation rewrote mailto/body bytes. No acceptance claim was made. Deployment fix adds explicit 308 clean-URL redirects and no-transform cache directives, preserving approved source hashes while preventing edge rewriting.

## Precondition Resources
- [p1-store-compliance-flows.puml](file://TASK-260712-1x0lot/p1-store-compliance-flows.puml) — Policy, moderation and reviewer flow context

## Outcome Resources
- [p1-policy-support-publication.md](file://TASK-260712-1x0lot/p1-policy-support-publication.md) — Stable/versioned route contract, source approval, product wiring, no-transform cache policy, live hash gate and rollback procedure
