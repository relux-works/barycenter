# Self-contained Pulsar Audio engineering

## Description
Deliver self-contained Pulsar Audio engineering for clips, Air, streamed tracks, near-live PTT, capture quality, soundboard and safe automation, plus an audit-ready E2EE design packet. Independent E2EE design audit and all E2EE implementation are intentionally deferred to EPIC-260716-3qsztl; hands-on real-app and hardware acceptance remains in EPIC-260714-th54l3.

## Scope
Own coordinator, Windows Pulsar, macOS Pulsar, Telegram adapter, protocol, persistence, non-E2EE media pipeline, privacy and UGC operations, migrations, rollout tooling and best-effort evidence required by docs/spec-self-contained-audio.md. Exclude the 19 manual physical, production-shaped and multi-day acceptance tasks and exclude TASK-260712-aniuyy plus all post-audit E2EE implementation and implementation-review work now owned by EPIC-260716-3qsztl. Retain only the four-task audit-ready E2EE design packet.

## Acceptance Criteria
All in-scope implementation and review tasks are complete; unit, integration, golden, migration, security, cross-build and package checks available to CI or development hosts are green; unsupported environments and unverified hardware claims are explicit; the four-task E2EE design packet is ready for independent audit. Engineering completion here must not be described as E2EE implementation, Store, hardware, beta or production acceptance. Those claims require EPIC-260716-3qsztl and EPIC-260714-th54l3 as applicable.
