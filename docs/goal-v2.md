# Goal v2.0 — Releasable macOS version (no Windows yet)

Target: a stranger reaches a shared synchronized broadcast with zero
terminal/configs. Distribution = ONE artifact (Pulsar.app) + pairing through
the bot. Everything that already exists must be re-verified from scratch.

## Definition of Done — end-to-end scenarios (each from a clean slate)

- **D1 Creator onboarding**: stranger → @barycenter_bot /start → /create →
  orbit + pairing code → installs Pulsar (dmg) → first-launch window asks for
  the code → paired (credentials in Keychain) → "Pulsar A" appears in the
  phone's Spotify → picks it → zeroconf login persists → track plays (solo).
  No terminal, no yml, no brew at any step.
- **D2 Partner onboarding**: invite link → companion → /pair → second Mac
  through the same window → both homes online → offline gate releases parked
  material automatically.
- **D3 Shared air**: selecting a track on either Pulsar → synchronized start
  without sending a bot link (measured desync ≤ 100 ms in the journal); bot
  links remain an optional queue; /pause /resume /skip /sync behave per spec;
  playlist/album link becomes the base flow with insert-and-return; voice
  notes: default personal, «всем» broadcast, boundary insertion works both modes.
- **D4 Takeover**: phone playback on a Pulsar in shared → policy user: selected
  track becomes the shared air; /takeover coordinator: busy air is restored.
  An idle shared air always adopts the selection. Both proven live.
- **D5 Resilience**: daemon restart needs no phone (persisted creds); Pulsar
  relaunch re-registers and continues; coordinator redeploy → session comes
  back PAUSED with queue intact → /resume; node offline → DEGRADED → auto
  or /resume recovery; speaker vanishes → auto-restore to configured output.
- **D6 Single-artifact distribution**: Pulsar.app bundles the relux-works
  go-librespot fork (universal or arm64), Developer ID signed, notarized,
  stapled; Sparkle auto-update channel live; GitHub release carries SLSA
  attestations; TestFlight build ships in parallel. Install = drag to
  /Applications, nothing else.
- **D7 Minimal UI**: onboarding window (code entry + progress + human
  errors); menu-bar item: connection state, now playing, output-device
  picker, login-item toggle, version+update check. No dock clutter.
- **D8 Ops hygiene**: prod healthz monitored; media retention sweep verified;
  /revoke + re-pair cycle works; bot texts stay clean HTML.

## Gates

- **R0 Revision** — full manual pass of TODAY's stack against D1–D5 on the
  dev stand (fresh orbit on prod, Ivan's node via --pair for now); every
  deviation logged in .temp/tasks.md and fixed or ticketed. Exit: checklist
  green with evidence.
- **R1 Self-contained app** — daemon bundled (no brew), built-in config
  defaults (zero yml for users), credentials move to Keychain, `--pair` flow
  reachable without terminal. Exit: clean Mac (or fresh user account) runs
  D1 with a dmg built by CI.
- **R2 UI** — D7 implemented (SwiftUI onboarding + menu bar over untouched
  NodeCore). Exit: D1/D2 pass mouse-only.
- **R3 Distribution** — D6 pipeline: sign → notarize → staple → attest →
  Sparkle appcast; TestFlight internal build. Exit: update from vN to vN+1
  arrives over Sparkle on a test Mac; `gh attestation verify` passes.
- **R4 Acceptance** — Katya's real onboarding (D2) + one evening of real use
  across two homes; desync journal reviewed; no critical bugs for 7 days.

## Invariants

Secrets never in repo/logs; Spotify account credentials never leave the
node's Mac; protocol v1 golden set changes only with paired codec+test
updates; prod orbits survive every migration (schema forward-compatible);
signed commits, no AI attribution; spec/docs English, bot UX Russian;
every claim of progress carries evidence (log path / test / screenshot).

## Out of scope

Windows app (fork groundwork only), Mac App Store release (TestFlight is
enough for beta), roles UI beyond /make_primary//revoke, group-chat binding,
scale beyond dozens of orbits, listener-role polish (M3), landing page.

## Needs from the customer

Developer ID certificate + App Store Connect access (by R3); Katya's Mac
session (R4); taste feedback on UI texts (R2).
