# Phase 1 policy and support publication

- Date: 2026-07-14
- Task: `TASK-260712-1x0lot`
- Source pack: `docs/compliance/policy-pack-2026-07-14.json`
- Site repository: `relux-works/pulsar-site` (Cloudflare Pages)
- Current state: **source-approved / deployment in progress**. Ivan Oparin
  approved the exact EN/RU source hashes from commit
  `43c0bd992e25c1e85aba6b7a086a94dad378eb35`, including the five new support
  sections, for production publication on 2026-07-14.

## Route contract

The approved canonical URL is the English controlling fallback. Russian is an
explicit stable locale path, so a packaged app, Store listing or reviewer never
depends on browser-language JavaScript.

| Document | English stable | Russian stable |
| --- | --- | --- |
| Privacy | `https://barycenter.live/legal/privacy` | `https://barycenter.live/legal/privacy/ru` |
| Terms | `https://barycenter.live/legal/terms` | `https://barycenter.live/legal/terms/ru` |
| Content Guidelines | `https://barycenter.live/legal/content-guidelines` | `https://barycenter.live/legal/content-guidelines/ru` |
| Upload/recording rights | `https://barycenter.live/legal/upload-rights` | `https://barycenter.live/legal/upload-rights/ru` |
| Support and safety | `https://barycenter.live/legal/support` | `https://barycenter.live/legal/support/ru` |

Every route also has an immutable archive at
`/legal/versions/1.0/{en|ru}/{document}`. The generated page contains the pack
version, effective date, full source SHA-256, stable section anchors and an
EN/RU switch. The public deployment manifest at
`/legal/deployment-manifest.json` binds every stable and versioned HTML body to
its exact source and rendered SHA-256.

## Product and Store wiring

- macOS exposes all five Russian routes from the menu-bar policy/support
  submenu; `NodeCore` tests pin the HTTPS host and paths;
- Windows exposes the same routes from the tray and portable Go tests pin the
  link contract;
- Telegram `/help` publishes the same five Russian links and its unit test
  prevents removal;
- `docs/compliance/store-public-links.json` is the EN/RU Store metadata source;
- the generated website navigation links every document and locale without
  authentication.

No real packaged-app click or physical-hardware observation is claimed here.
Those observations remain in the manual-test epic; deterministic constants,
rendering, link structure and live unauthenticated HTTPS are engineering scope.

## Cache, deployment and rollback

Stable routes use `Cache-Control: public, max-age=300, must-revalidate` so a
correction converges quickly. Immutable version routes use one-year immutable
caching. The deployment manifest is `no-store` and is the certification probe's
entry point.

Cloudflare Pages deploys `pulsar-site/main`. Its pull-request workflow checks
out the exact upstream Barycenter commit named in the deployment manifest,
regenerates the complete legal bundle and rejects any byte mismatch. It also
requires the source pack to be `proceed`, preventing a staged/held branch from
being published through the normal merge path.

Rollback is a Git revert or redeploy of the last accepted `pulsar-site` commit.
Previously published immutable version routes must not be deleted. After a
rollback or forward correction, run `policy-site-check --require-proceed
--live`; it requires HTTP 200 without authentication, exact page hashes,
source-hash metadata and the cache contract for all 20 routes. Store submission
and the scheduled uptime workflow run the same live gate.

## Remaining deployment gate

The exact source approval is recorded as `proceed`. The task is not accepted
until the generated `ready` bundle lands on `pulsar-site/main` and the
production live checker proves every route, byte hash and cache header. Store
submission continues to fail before upload until that deployment evidence
exists.
