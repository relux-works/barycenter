# duet Runbook

Deploy, operate and debug duet without reading the sources (goal §2). Spec: `docs/spec.md` v1.2. All commands assume the release artifacts from `make release VERSION=vX.Y.Z`: `NodeApp-vX.Y.Z.app.zip`, `duet-coordinator-vX.Y.Z-linux-amd64`, `checksums.txt`, plus the `deploy/` directory.

## 1. Fresh node install (home Mac)

Prereqs: Mac on macOS 14+, prepared per spec ch. 12 (autologin `duet` user, no sleep, manual OS updates, hostname `node-a`/`node-b`), tailscale up (`tailscale status`), brew installed.

```bash
brew install go-librespot
# copy NodeApp-vX.Y.Z.app.zip and the deploy/ dir to the Mac, then:
cd deploy && ./install-node.sh ~/Downloads/NodeApp-vX.Y.Z.app.zip
```

Then edit `~/duet/node.yml`: `node_id`, `coordinator.token` (from the coordinator config), `librespot.device_name`, `airfoil.speakers`. Restart the agent:

```bash
launchctl kickstart -k gui/$(id -u)/works.relux.duet.nodeapp
tail -f ~/duet/nodeapp.log        # expect "welcome received"
```

Check from any tailnet machine: `curl http://coord:8080/healthz` shows `"<node>": true`.

Note: the LaunchAgent points inside `NodeApp.app` (the .app bundle is required for Airfoil capture, spec 6.3); spec B.1's bare-binary path predates that and the template here is authoritative.

## 2. Fresh VPS install (coordinator)

Prereqs: Ubuntu 24.04, tailscale joined as `coord` (spec ch. 11), no public inbound ports (`ufw default deny incoming; ufw allow in on tailscale0; ufw enable`).

```bash
apt install -y ffmpeg sqlite3
# copy the linux binary and deploy/ dir, then:
cd deploy && sudo ./install-coordinator.sh ./duet-coordinator-vX.Y.Z-linux-amd64
sudo nano /etc/duet/coordinator.yml    # tailscale IP, two tokens (openssl rand -hex 32), telegram
sudo systemctl restart duet-coordinator
journalctl -u duet-coordinator -f      # expect "listening"
```

Daily DB backup (weekly rotation, spec ch. 16): add to `duet` user's cron:
`0 4 * * * sqlite3 /var/lib/duet/duet.db ".backup /var/lib/duet/backup/duet-$(date +\%u).db"`

## 3. First Spotify login (per home, spec ch. 13)

1. Stop NodeApp: `launchctl bootout gui/$(id -u)/works.relux.duet.nodeapp`.
2. Run the daemon by hand: `/opt/homebrew/opt/go-librespot/bin/go-librespot --config_dir ~/"Library/Application Support/go-librespot"` (NodeApp has already rendered config.yml there on its first start; if the dir is empty, start NodeApp once first).
3. The account owner opens Spotify on their phone (same Wi-Fi) and picks the device ("Дом A"/"Дом B"). Credentials are saved by the daemon.
4. Ctrl-C the daemon, start NodeApp back: `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/works.relux.duet.nodeapp.plist`.

Check: `curl http://127.0.0.1:3678/` returns `{"playback_ready":true}` shortly after start.

## 4. Speaker delivery setup (per home, spec ch. 14, v1.3)

### 4a. Direct mode (default, `airfoil.enabled: false`)

1. Pair/connect the home's device (AirPlay speaker, Bluetooth or wired).
2. Pick it as the output in Control Center once; copy its exact name into `node.yml` -> `audio.output_device`.
3. NodeApp keeps the output on that device: if it drops, macOS falls back to built-in speakers, NodeApp reports degraded and re-selects the device automatically as soon as it returns (proven live: forced drift restored within one poll).
4. Nothing else to install or license.

### 4b. Airfoil mode (`airfoil.enabled: true`, macOS 15 Sequoia ONLY)

macOS 26 Tahoe warning: Airfoil 5.12.6 produces severe dropouts in every capture configuration on Tahoe (live bisection 2026-07-03) — do not enable there.

1. Install Airfoil (Rogue Amoeba), allow the ACE system extension (System Settings prompts + possible reboot). License: trial injects noise after ~10 min — fine for testing in <=8 min windows, license required for production.
2. In Airfoil: disable **Silence Monitor**; connect the home speakers; fix audible intra-home skew with per-speaker Sync (Advanced Speaker Options). Copy exact speaker names into `node.yml` `airfoil.speakers`.
3. First NodeApp start with Airfoil present triggers the Automation permission prompt (NodeApp -> Airfoil). Approve it; without it the AppleScript bridge cannot run.
4. macOS 26 (Tahoe) warning: Airfoil 5.12.6 has a known sample-rate-mismatch bug losing audio on external devices; if speakers are silent while NodeApp plays, pin the output device to 44.1 kHz (Audio MIDI Setup) and re-test.

## 5. Offset calibration (both homes on a call, spec ch. 14)

```
/offset a 0
/offset b 0
/offset_test          # 5 synchronized clicks
```

Whoever's click lags gets a bigger offset (offset = start earlier): `/offset b 300`, re-run `/offset_test`, step 100 ms then 25 ms until clicks merge (< ~50 ms echo). Values persist in the coordinator DB and are pushed to nodes on every reconnect. Re-calibrate after speaker/firmware/Airfoil changes; `/sync` mid-track is the quick fix for drift.

## 6. Diagnostics: symptom -> action

| # | Symptom | Action |
|---|---|---|
| 1 | `/status` says a node is offline | On that Mac: `tailscale status` (tailnet up?), `launchctl print gui/$(id -u)/works.relux.duet.nodeapp` (agent running?), `tail ~/duet/nodeapp.log` (config error prints an explicit reason; bad token shows "register rejected" on the coordinator). Reboot recovers automatically: agent is RunAtLoad+KeepAlive |
| 2 | librespot restart-cycling (`librespot_restart` errors in chat/log) | `grep librespot ~/duet/nodeapp.log \| tail -30`. Usual causes: lost credentials (redo first login, §3), port 3678 busy (`lsof -i :3678`), broken FIFO path (NodeApp recreates it — check `audio.fifo_path` dir exists) |
| 3 | One speaker silent, chat says degraded | Airfoil lost it. NodeApp reconnects with backoff (<=60 s after power-on). If it never returns: open Airfoil, check the speaker's name still matches `airfoil.speakers` exactly (renames break matching) |
| 4 | All speakers silent, NodeApp says playing | Airfoil Automation permission revoked or ACE broken after a macOS update: System Settings -> Privacy & Security -> Automation -> NodeApp -> Airfoil ON; reinstall ACE from Airfoil if prompted. Tahoe: see §4 item 4 (sample-rate bug). Last resort: Airfoil source = NodeApp manually |
| 5 | Homes audibly out of sync | `/sync` first (restarts current track synced). Recurs: re-run §5 calibration; check `/status` rtt (tailnet detour: `tailscale ping node-b`) and that both Macs run NTP (`sntp -sS time.apple.com`) |
| 6 | Voice message never plays | Chat shows processing errors? `journalctl -u duet-coordinator \| grep -E "voice\|media" \| tail`. ffmpeg present on VPS (`ffmpeg -version`)? Node side: `grep media_download ~/duet/nodeapp.log` — 401 means token mismatch, 404 means media expired (30 d retention) |
| 7 | Bot silent | `journalctl -u duet-coordinator \| grep -i telegram \| tail`. `getUpdates` errors = bad bot_token; commands ignored = your user id is not in `telegram.users` (bot ignores strangers silently by design); group links unseen = BotFather privacy mode must be Disabled |
| 8 | Coordinator down / VPS rebooted | systemd restarts it; after restart the session is PAUSED with a chat notice — `/resume` continues (position from node heartbeats). If unit is dead: `journalctl -u duet-coordinator -n 50`; config errors print explicit `config invalid:` lines |
| 9 | Track skipped with "недоступен" | Region mismatch between the two accounts (spec 4.4) — expected per-track; if systematic, align account regions |
| 10 | Music stutters, `audio_starvation` errors | Wi-Fi or Spotify connectivity on that Mac; NodeApp soft-restarts the daemon automatically. Check `underruns` growth in `/status` and the Mac's network |

## 6a. Build machine: signing identity (once, before the first release)

`scripts/setup-signing.sh` creates the self-signed identity `duet-nodeapp` in a
dedicated keychain; `make app`/`make release` pick it up automatically. All
releases must be signed with the same identity — that is what keeps the TCC
Automation grant alive across updates (goal DoD-2). Losing the keychain =
users re-approve NodeApp -> Airfoil once after the next update; the identity's
designated requirement is printed at every build for verification.

## 7. Update to a new version

Artifacts are versioned; state (SQLite, node.yml, coordinator.yml, offsets, volumes, queue) survives (goal §2.2).

- Node: `./install-node.sh NodeApp-vNEW.app.zip` (keeps node.yml, restarts agent). TCC Automation persists across updates thanks to the stable signing identity + bundle id.
- VPS: `sudo ./install-coordinator.sh duet-coordinator-vNEW-linux-amd64` (keeps config and DB; session comes back PAUSED, `/resume`).

## 8. Production intro (after licenses are bought)

1. Buy two Airfoil licenses (one per Mac), enter them, re-verify §4.
2. Move node-b hardware to home B; re-join tailnet, re-run §3 login for account B on its home Wi-Fi.
3. Re-run §5 calibration across the real homes.
4. Multi-hour stability soak (spec ch. 20 risk): one evening of shared mode; watch `/status` underruns/degraded and the desync journal.
5. Acceptance checklist re-run: spec ch. 18 items on the real setup (`docs/acceptance-run.md`).
