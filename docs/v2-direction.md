# v2 Direction — production-grade for two (customer decision 2026-07-03)

Source of truth from the customer, verbatim intent: the service must be
distributed like an adult product because the second user (Katya) must never
touch configs, terminals or "system administration of her Pulsar". Not
thousands-of-users scale — but production-ready, "because our relationship is
production-ready". This document supersedes the conflicting parts of spec v1.3
and goal v1.0 until a consolidated spec v2 is written.

## What changes

| Area | v1.x | v2 |
|---|---|---|
| Coordinator hosting | VPS in tailnet, no public ports | relux.works infra (Coolify on `colima-coolify` VM, Mac mini host), subdomain (proposed `barycenter.relux.works`), TLS/wss via the existing reverse proxy; node auth = per-node tokens over TLS. The "zero public ports / tailnet-only" invariant is consciously retired for nodes |
| Node networking | Tailscale required on both Macs | Not required: nodes dial `wss://` over the public internet |
| Onboarding | Hand-edited node.yml + tokens | Pairing flow: `/pair` in the bot issues a one-time code; the app asks for the code on first launch and receives node_id + token + ws URL from the coordinator. Zero config files for the user |
| App | SwiftPM CLI in a minimal bundle, install script | Real macOS app (SwiftUI shell over the existing NodeCore engine): onboarding, status, output-device picker. Auto-updating distribution |
| App distribution | zip + install-node.sh | Beta: public link; then store-grade updates. **Mine: go-librespot is GPL-3.0 — App Store/TestFlight distribution is legally hostile to GPL (VLC precedent) and App Store sandbox fights our child daemon + default-output switching. Working proposal: Developer ID + notarization + Sparkle auto-updates (same seamless UX); revisit store if the GPL dependency is ever replaced** |
| Bundle id | works.relux.duet.nodeapp | `works.relux.pulsar` (naming: app = Pulsar, coordinator = Barycenter, modes = periastron/apoastron). TCC/signing identity resets — free now, nobody is in production yet |
| Secrets | yml files chmod 600 | Env-var overrides for all secrets (Coolify-style), yml keeps non-secret defaults |

## Kept from v1.x (unchanged)

Protocol v1 (26 golden), sync mechanics (two-step load, resume_at/T_local,
audible_position, ring/backpressure), direct delivery mode default +
airfoil flag (Sequoia-only), media pipeline, session FSM + playlist layer
(U10), takeover policy (U9, default user), NodeCore engine and its test suite.

## Migration plan (gates v2)

1. **V2-G1 Coordinator on relux**: Dockerfile (alpine + ffmpeg), env-based
   secrets, /healthz behind the proxy, Coolify app + subdomain, bot live with
   the production token. Deploy path: TBD with the customer (git remote for
   Coolify to pull, or a registry image).
2. **V2-G2 Pairing**: coordinator API `POST /pair {code}` -> `{node_id, token,
   ws_url}`; `/pair` bot command issues one-time codes (TTL 5 min, one per
   free slot); dev-Mac Pulsar re-onboards through it as the proof.
3. **V2-G3 Pulsar.app**: SwiftUI shell (onboarding with code, status window,
   output picker, login item), bundled go-librespot (no brew), Developer ID +
   notarization + Sparkle appcast; beta link for Katya.
4. **V2-G4**: acceptance re-run across two homes on the v2 stack.

## Amendments 2026-07-04 (customer)

- Playlist end behavior: **stop + notice** (confirmed; already implemented).
- Distribution: **TestFlight internal beta + notarized/Sparkle as the parallel
  channel** (two-channel is the norm). GPL note stays on record: VLC re-entered
  the store only after relicensing to LGPL/MPL; go-librespot is GPL-3.0 —
  store-release options: upstream dual-license request, or first-run component
  download keeping the store binary GPL-free. TestFlight beta proceeds.
- **Windows Pulsar research** commissioned: maximum-sandbox posture ("access
  to nothing"), Store-grade distribution. Working hypothesis: MSIX +
  AppContainer + WASAPI; open: child daemon inside AppContainer, background
  autostart. Deep-research pass pending.
- **Multi-tenant + shared orbits**: see docs/v2-multitenant-design.md — one
  coordinator hosts dozens of "barycenters" (orbits), pairing/invites via bot,
  N pulsars per orbit down an invite chain (polyamory/friends contexts),
  roles host/partner(/listener). Supersedes the "exactly two homes" hard
  limit of spec v1. Git flow: push to the customer's relux.works git with
  the customer's signatures (remote URL pending).

## Open items for the customer

- Subdomain name: barycenter.relux.works? (bot: @periastron_bot or similar)
- Coolify deploy source: push this repo to a git remote Coolify can pull, or
  build/push docker images to a registry? (need the preferred flow)
- Developer ID cert available for notarization (needed at V2-G3).
- Telegram bot token (pending) + Katya's telegram user id when ready.
