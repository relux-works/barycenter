# Unresolved Questions

Open decisions, their owners, and where the answer must land. Spike-answerable items also tracked in `.temp/tasks.md` (Phase 0).

## Spec v1.2 review remarks (agent, 2026-07-03) — awaiting customer verdict

Per goal §7.6: divergences and spec-internal conflicts are recorded here, never silently reinterpreted. None of these blocks Phase 1 gates G0-G4 (R3 concerns solo voice = Phase 2), but R3/R4 need a spec verdict before Phase 2 / the affected code is written.

| # | Remark | Proposal |
|---|---|---|
| R1 | `audible_position = librespot_position - ring_fill_ms` (spec 6.3) omits the kernel pipe buffer (16-64 KB ≈ 45-190 ms at 352.8 KB/s) — systematic overestimate | Accept as known tolerance for heartbeat, or add a constant estimate; decide when solo threshold work starts |
| R2 | "resume -> first samples goes into calibrated offset" (spec 6.3) assumes that delay is constant; Spotify stream start is network/cache dependent. If jitter is large, the 150 ms target fails | Spike remainder: measure resume->first-sample spread over ~10 starts. Plan B (recorded, not adopted): early resume at T-800ms + render gate opening the ring read exactly at T_local |
| R3 | Solo voice (4.2 + 6.3): threshold "400 ms of *audible* remainder" fires when the daemon is already ~ring_fill inside the NEXT track (gapless pipe write). Then `pause` stops the daemon mid-next-track, the ring plays a chunk of the next track's start, and `next` skips that track entirely | Count the 400 ms threshold from the DAEMON position (as v1.1 effectively did), then after pause wait for ring drain, play the insert, then `next`. audible_position stays correct for heartbeat//now. Phase 2 scope |
| R4 | 6.6 "underruns > 3 s -> audio_starvation + soft librespot restart" conflicts with "daemon writes nothing while paused": every long PAUSED/wait period would trigger restarts | Gate starvation detection on "node expects music to be playing" state |

## Spec v1.3 amendments from live spike probes (2026-07-03, logged-in session)

| # | Finding | Proposed spec change |
|---|---|---|
| A1 | zeroconf credentials are NOT persisted by default in 0.7.4; daemon restart requires the phone again. `credentials.zeroconf.persist_credentials: true` fixes it (verified live: creds on disk after next login) | Add the key to A.2 config example; ch. 13 checklist gains "verify state.json contains stored credentials after first login" |
| A2 | `play` of an unavailable/nonexistent uri: HTTP 500 + spurious `will_play(bad uri)` + `/status` track goes null while old audio keeps flowing | 4.4/6.2: node maps load HTTP failure to `error(load_failed)` right away and distrusts `/status` until the next successful play |
| A3 | No `not_playing` on track-to-track transitions (only `will_play`/`playing` of the next); `stopped` fires at end of context | 6.3.5 wording: ended trigger = `stopped`/`not_playing` where emitted; single-track plays (shared cycle) do produce it; `will_play(new)` is the solo boundary marker |
| A4 | Daemon `/status` position advances on wall-clock, not bytes delivered; slow consumers let position outrun audio beyond ring_fill | Note under 6.3 audible_position: valid while output consumes realtime; sustained output stalls add drift (diagnostics: /status vs heartbeat position divergence) |
| A5 | R2 measured: resume->first-data 37-42 ms (±3 ms) on loaded tracks; fresh network start 0.7-1.3 s happens only in the load phase | 6.3: current anchor design confirmed; no plan-B render-gate needed for MVP |

## Protocol v1 additions (agent, 2026-07-03) — propose folding into spec v1.3

Spec 9.1 requires `/offset` to be *pushed to the node* and `/offset_test` to fire synchronized clicks on both nodes, but the ch. 8 message catalog has no carrier for either. Added to protocol v1 with golden files in the same change (goal invariant 5): `set_offset {offset_ms}` and `offset_test {t_coord_ms, clicks, interval_ms}` (coordinator -> node). See docs/protocol.md. Spec ch. 8.3 table should gain both rows in v1.3.

## Goal DoD items unreachable autonomously (per goal "СВЕРКА": recorded with proposals)

| DoD | Blocker | Proposal |
|---|---|---|
| DoD-1 tag v1.0.0 | Two-fold: (a) user's standing rule "never commit/stage automatically" — the tree is entirely uncommitted, a tag needs commits; (b) semantically the tag belongs after G3 acceptance | User commits/tags himself (or explicitly delegates); tag after acceptance passes |
| DoD-5/S5 AirfoilBridge live dictionary check | Airfoil not installed (ACE approval + possible reboot = user action) | 15-min session right after Airfoil install: run NodeApp, approve Automation prompt, verify source/connect/poll against the real dictionary |
| DoD-7 "speaker unplugged" leg | Needs Airfoil + a physical speaker | Same Airfoil session. The other two legs (kill librespot, node network cut) are reproduced live — see .temp/tasks.md G2 evidence |
| DoD-8 acceptance, DoD-10 clean install | Need G0 login, Airfoil, second Mac/account (U5/U6), VPS (U2) | Run per docs/acceptance-run.md once hardware exists; clean install on a fresh macOS user per runbook §1 |

## U9 — shared mode vs partner's phone — **DECIDED 2026-07-03 (customer)**

Configurable takeover policy, set via bot command `/takeover <user|coordinator>` (persisted in coordinator settings):
- `user`: the phone wins — global switch to solo + chat notice "дом X забрал управление — режим solo".
- `coordinator`: the station wins — node softly stops the intruding playback, coordinator restores the broadcast element on that node only (live position, no restart for the other home) + chat notice "дом X вмешался с телефона — эфир восстановлен".
Mechanics: node in shared detects daemon playback with uri not matching the current element -> new protocol message `external_playback {uri}` -> coordinator applies the policy. Default policy: coordinator (proposed, awaiting confirmation). Spec v1.4 note pending.

## U10 — shared playlist layer — **DECIDED 2026-07-03 (customer), implementation in G2**

Two-layer broadcast queue: base = active shared playlist (Spotify playlist/album link expanded into a track list, played track-by-track through the normal sync cycle, cursor persisted; a new playlist link replaces the active one with a notice); overlay = inserted singles/voices that play right after the current track, after which the broadcast returns to the playlist at the NEXT track after the interruption point (boundaries only, no mid-track cuts). Empty overlay + finished/absent playlist -> chat notice instead of silent dead air. Requires Spotify Web API (client credentials) to expand playlists: customer to register an app (developer.spotify.com) and provide client_id/secret for coordinator.yml. Open sub-questions: default takeover policy confirmation; playlist end behavior (stop+notice vs loop); private playlists need user OAuth (start with public/link-shared ones).

## New since v1.2 (agent findings)

| # | Question | Context | Blocks |
|---|---|---|---|
| U7 | Airfoil 5.12.6 (2025-09-02, latest as of 2026-07-03) has only "initial support" for macOS 26 Tahoe with a KNOWN BUG: lost audio from sample-rate mismatches when capturing to external devices — exactly our path (NodeApp -> AirPlay speakers). Rogue Amoeba advises delaying Tahoe upgrades; dev Mac runs 26.5 | Release notes rogueamoeba.com/airfoil/mac/releasenotes.php; spec ch. 20 risk updated reality | **CONFIRMED LIVE 2026-07-03 (see U8)** — rate pinning to 44.1 kHz did NOT help |

## U8 — DECISION NEEDED: speaker delivery on macOS 26 (spec key-decision change, customer sign-off required)

Live bisection (2026-07-03, dev Mac 26.5): bare chain Spotify->daemon->FIFO->NodeApp->Mac speakers = clean; direct AirPlay via macOS default output (Control Center pick) = clean; ANY Airfoil path (app-source->Computer, System-Wide->AirPlay, rates aligned at 44.1) = severe dropouts. Airfoil 5.12.6 is unusable for production audio on Tahoe. Additional S5 facts: scripted app-source setter broken (missing value) in 5.12.6; System-Wide source works scripted but duplicates local output (mute required) and forbids Computer output (feedback guard).

| Option | Trade-offs |
|---|---|
| (a) Node Macs stay on macOS 15 Sequoia | Spec unchanged (multi-speaker via Airfoil), Airfoil stable there; requires keeping node hardware off Tahoe until Apple/RA fix |
| (b) **MVP: direct AirPlay, one speaker per home, no Airfoil** (recommended) | Clean on Tahoe (proven); zero Airfoil licenses; speaker picked once in Control Center at setup; NodeApp monitors default-output name for degradation; multi-speaker (1..N) postponed; AirfoilBridge code stays behind a config flag `airfoil.enabled` for Sequoia nodes / post-fix |
| (c) Wait for Apple's Tahoe audio-capture fix | Unbounded timeline; blocks G2/G3 audio quality |
| (d) Multi-Output aggregate device with AirPlay | AirPlay devices are not enumerable in CoreAudio until activated; fragile, not recommended |

**DECIDED 2026-07-03 (customer): both modes supported, default = direct (no Airfoil).** Spec updated to v1.3 (ch. 2 key decision, 6.2.8, 6.4, 14.0/14.1, A.1). Implemented: `airfoil.enabled` flag (default false), `audio.output_device`, DirectOutputMonitor (poll + auto-restore + degraded heartbeat; live-proven: forced drift to built-in speakers was auto-restored to "Tima's JBL" within one poll). Airfoil path remains for macOS 15 nodes.

## Needs the customer (Ivan)

| # | Question | Context | Blocks |
|---|---|---|---|
| U1 | Airfoil licenses (2x, one per Mac) — when to purchase? Trial injects noise after 10 min, unusable for spike S4-S6 and production. | Spec ch. 14, ch. 20 | Spike S4-S6, Phase 1 acceptance |
| U2 | Which VPS (provider/region) for the coordinator? Needs Ubuntu 24.04, 1 vCPU / 1 GB, inside tailnet. | Spec ch. 16 | Phase 1 deploy |
| U3 | Telegram: one group (A + B + bot, base variant per spec) or two private chats? | Spec ch. 9 | Phase 1 bot config |
| U4 | Spotify account regions for A and B — do catalogs overlap enough? Pick probe tracks for acceptance item 9. | Spec ch. 15, 18.9 | Acceptance |
| U5 | Hardware for the two home nodes (Mac mini or what exactly), and whether THIS Mac doubles as node-a during development. | Spec ch. 12 | Phase 1 setup |
| U6 | zeroconf login for spike: needs a Premium account owner's phone on the same Wi-Fi as the spike Mac. When can we do a 10-min session? | Spec ch. 13 | Spike S1 (live API), S2, S6 |

## Spike must answer (Phase 0, spec ch. 19-20)

| # | Question | Where the answer lands |
|---|---|---|
| Q1 | go-librespot API: load in paused state at position? add-to-queue? boundary events and their latency? behavior on unavailable track? | docs/spike-report.md + spec 6.2/8.3 decisions |
| Q2 | FIFO on pause: stream stalls or silence flows? On track change? | docs/spike-report.md, NodeApp ring buffer design |
| Q3 | Does Airfoil capture a bare CLI binary or is .app bundle mandatory? | docs/spike-report.md, NodeApp packaging |
| Q4 | Does go-librespot ever emit non-44100 sample rates (need AVAudioConverter or not)? | docs/spike-report.md |
| Q5 | solo_inject: queue API available -> exact endpoint; not available -> fallback ("replace at boundary") or defer feature? | spec 8.3 update, Phase 2 scope |
| Q6 | Airfoil current version vs macOS 26.x compatibility (spec cites coverage up to macOS 26; this Mac runs 26.5). | docs/spike-report.md, U1 purchase decision |

## Decided (log)

| Date | Decision |
|---|---|
| 2026-07-03 | Spec v1.1 accepted as source of truth; repo layout: monorepo `node-app/` + `coordinator/` + `spike/` |
| 2026-07-03 | Q1/Q5 (docs-level, live confirm pending): go-librespot 0.7.4 has `POST /player/add_to_queue` -> solo_inject direct; `play` supports `paused:true` but has no position field -> load = play(paused) + seek(position); health endpoint = `GET /` (`playback_ready`) |
| 2026-07-03 | Q2 (partial): daemon start does not block on FIFO (lazy open) — NodeApp reader may start anytime; pause behavior still pending login |
| 2026-07-03 | NodeApp reader design: block on full ring (backpressure), never drop; lock-free SPSC ring in production (spike S3 finding) |
