# Goal v2.1 — Fix the beta findings, reach Katya-ready (Windows-first)

Target: everything Timur's live beta exposed is fixed; approaches feel
finished; updates and pairing never surprise; the Windows app reaches the
same onboarding quality as macOS — because Katya is on Windows.

## Definition of Done

- **F1 Living air (L1.5)**: linked sessions start when each side has ≥1 home
  online; offline homes never block and CATCH UP with a live-position join;
  on /accept the code-issuer's playing stream continues onto all homes (no
  dialogs; issuer silent → acceptor's stream; both silent → blank). Strict
  gate stays for single orbits.
- **F2 Pairing that survives**: credentials move to Data Protection keychain
  with a TeamID access group — Sparkle updates NEVER drop pairing (decision
  documented in docs). Legacy ~/duet/node.yml auto-retired with a log line.
- **F3 Re-pair UX**: menu-bar "Подключить заново…" opens the onboarding
  window anytime; bot side: /rebind issues a code and revokes the old slot
  token on successful re-pair; menu bar shows coordinator host + slot@orbit.
- **F4 Update hygiene**: Sparkle silent auto-update verified live vN→vN+1
  (watchdog 5s + SUAutomaticallyUpdate shipped) — app relaunches itself, no
  stuck Updater, pairing intact (F2 proven by the same update).
- **F5 Zeroconf visibility**: Timur's invisible-speaker case root-caused
  (firewall hypothesis confirmed/refuted); onboarding window and guide warn
  about macOS firewall + same-Wi-Fi + VPN; if firewall — one-click hint.
- **F6 Windows Katya path**: pulsar-win runs live on Ivan's Windows machine:
  pair via code, daemon up, "Pulsar X" visible in Spotify, synced start in a
  linked air with a Mac home; onboarding window + tray menu (code entry,
  status, quit) matching mac UX; MSIX package built in CI; Store submission
  ready (Partner Center verified, listing drafted).
- **F7 Beta cycle proof**: one full evening with Ivan+Timur on the fixed
  build: approach → living air with a dead home present → catch-up join →
  voice notes both ways → /apart — zero raw ids, zero stuck states, desync
  logged ≤100ms on two accounts.

## Gates

- **B1 Core fixes**: F1 done with tests (gate/catch-up/transplant); F2+F3
  implemented; all suites green, deployed to prod.
- **B2 Update proof**: tag a beta; Ivan's app silently self-updates and
  keeps pairing (F4); release notes list fixes.
- **B3 Zeroconf & polish**: F5 closed with Timur; guide/FAQ updated; menu
  shows connection identity.
- **B4 Windows live**: F6 on real hardware — blind-risk list burned down
  (WASAPI, pipe, timings by ear); fixes committed same-day.
- **B5 Acceptance**: F7 evening passes; then Katya's Windows onboarding is
  scheduled (out of this goal: her actual evening).

## Invariants

Trunk-based to prod behind compatibility (linkless/legacy behavior
unchanged); secrets never in repo/logs; protocol changes ship with golden +
both codecs (+ pulsar-win mirror); every fix lands with a test or a live
log as evidence; commits signed, no AI attribution; bot UX Russian, docs
English; production orbits survive every migration.

## Needs from the customer

Windows machine online (B4); Partner Center registration progressing (F6);
Timur's firewall check result (F5); an evening with Timur (B5).
-- these are ouside the goal for now and tasks above which require these shoud be excluded 