# Spike Report — Phase 0

Status: **S1-S3 complete (doc research + live logged-in probes 2026-07-03); S4-S6 pending Airfoil install.**
Machine: arm64 Mac, macOS 26.5, Swift 6.3.2, go-librespot 0.7.4 (brew bottle `arm64_tahoe`).
Live-probe artifacts: `.temp/spike/` (pipe-meter.log, events.log, librespot-login*.log). Instruments: `pipe-meter.py` (paced FIFO consumer with gap/rate marks), `ws-events.py` (stdlib RFC6455 client for /events).
Spec references: ch. 19 (Phase 0 scope), ch. 20 (risks).

## S1. go-librespot API capability checklist — ALL CONFIRMED LIVE

| Capability (spec needs) | Verdict | Live evidence (2026-07-03) |
|---|---|---|
| Load track paused | **YES, live** | `POST /player/play {uri, paused:true}` -> `/status`: `paused:true`, new uri, `position:0`, **zero bytes into the pipe** |
| Seek while paused | **YES, live** | `POST /player/seek {position:30000}` in paused state -> `/status position:30000`, still paused, still no pipe data. Two-step load (spec 6.3) fully validated |
| Resume after two-step load | **YES, live** | resume -> first pipe bytes in **41 ms** |
| Resume->first-data jitter (R2) | **EXCELLENT** | 5x pause/resume series: 42/38/39/39/37 ms — jitter ±3 ms. The scary network-start delay (~0.7-1.3 s observed on fresh `play`) lives entirely in the load phase, which the shared cycle absorbs before `resume_at`. R2 concern closed for the coordinator's load->resume mechanics |
| Track boundary events | **YES, live** | Track-to-track transition (album context): `will_play(next)` -> `metadata(next)` -> `playing(next)` within ~350 ms; **no `not_playing` emitted on transition**. End of a single track / end of context: `stopped` observed. NodeApp ended-detection for shared (single-track plays): `stopped`/`not_playing`; `will_play(new uri)` is the boundary marker in continuous contexts (solo) |
| Position read | **YES, live** | `/status track.position` + full track object: `uri, name, artist_names[], album_name, album_cover_url, position, duration, release_date, track_number, disc_number`. Bot replies can use real titles via node metadata later (MVP: id, spec 9.1 allows) |
| Add to queue (solo_inject) | **YES, live** | `POST /player/add_to_queue` -> HTTP 200; at the next boundary the queued track **actually played, overriding album order**. solo_inject fully proven |
| Unavailable track | **CONFIRMED, ugly** | `play` of a nonexistent uri -> **HTTP 500**, plus daemon emits `will_play(bad uri)` and `/status` track object goes null while the previous stream keeps flowing; coincided with an AP reconnect, state stayed broken until `stop` + fresh `play`. NodeApp mapping: HTTP 500 on load -> `error(load_failed)` immediately, never trust `/status` until the next successful play (current `confirmPausedLoaded` already throws on 500 before polling) |
| Volume control | docs | `GET/POST /player/volume`; `external_volume: true` hands volume to us (unused by NodeApp mixer path) |
| Health endpoint | **YES, live** | `GET /` -> `{"playback_ready": bool}`; flips to true only when a Spotify session is active |

## S2. Daemon + pipe behavior — CONFIRMED LIVE

- **No pacing in the daemon: the reader dictates throughput.** A greedy reader drained decoded PCM at 54-73 MB/s (2.6 GB in seconds) and tracks "played" instantly, cascading track-to-track. Realtime tempo must come from NodeApp's render callback + ring backpressure — exactly the spec 6.3 design.
- **Pause = pipe stall** (no silence stream): zero bytes during pause windows. Spec v1.2's "librespot writes nothing while paused" assumption is now fact. Underrun-as-silence in NodeApp is the correct complement.
- **Killing the pipe reader kills playback**: daemon logs `output device failed: broken pipe` and the current track dies (that was the cascade trigger too). NodeApp's reader must run for the daemon's whole lifetime (our reopen-loop does); a NodeApp restart implies the current element restarts via the coordinator (spec 6.6 already says so).
- **`/status` position advances on wall-clock, not on bytes consumed.** With a slower-than-realtime reader, position outruns the pipe stream (measured 304 bytes per track-ms vs format's 352.8). With NodeApp's realtime consumption the skew is bounded by ring fill (audible_position formula holds), but any sustained output stall lets daemon position drift ahead — noted for diagnostics.
- **Format**: f32le confirmed indirectly — sustained pipe throughput matched the reader's ~305 KB/s ceiling (above s16le's total 176.4 KB/s, consistent with 44.1k f32 stereo 352.8 KB/s being writer-side available). Definitive audible check lands free with the first NodeApp play (wrong rate would chipmunk instantly). No non-44.1k tracks encountered.
- **Boundary continuity**: across a track-to-track transition the pipe stream had no gap >300 ms — near-gapless. Confirms remark R3: at an audible-threshold the daemon is already inside the next track; solo-voice interception must use the daemon position (R3 fix recorded in UNRESOLVED).
- **Zeroconf credentials are memory-only by default**: after a daemon restart the phone must re-select the device; `state.json` kept `credentials: empty`. **Fix confirmed live**: `credentials.zeroconf.persist_credentials: true` -> after the next phone login, credentials landed on disk (296 bytes for the account). NodeApp's config renderer now emits this key (test updated); spec A.2/ch.13 need the same line (UNRESOLVED, spec v1.3).
- Daemon resilience: an AP connection drop mid-session recovered with automatic re-authentication (no phone) while the process lived.

## S3. AVAudioEngine FIFO prototype (synthetic, first session)

- `spike/fifo-player/`: FIFO -> reader thread -> 1.0 s ring -> AVAudioSourceNode. PASS; underruns silent; EOF->reopen works; engine resamples 44.1k->48k itself.
- Design rule discovered and baked into spec v1.2 + NodeCore.RingBuffer: reader blocks on full ring (backpressure), never drops; production ring is lock-free SPSC (implemented and concurrency-tested in NodeCore).

## Decisions fed back into implementation

1. Two-step load (`play paused` + `seek`) — validated; `ready` after status confirms paused state (poll works live, confirmation arrives immediately).
2. solo_inject = `add_to_queue` — validated end-to-end including boundary playback.
3. `persist_credentials: true` — mandatory in the daemon config (renderer updated; spec A.2 amendment proposed).
4. Load failure = HTTP 500 -> `load_failed` immediately + treat `/status` as unreliable until next successful play.
5. ended detection (shared): `stopped`/`not_playing` + ring drain (spec 6.3.5); `will_play` is the solo boundary marker.
6. R2 closed: resume jitter ±3 ms on loaded tracks; started-detection by first nonzero sample stays (data beats the `playing` event by ~400 ms).

## S4-S6 — COMPLETE (live with Airfoil trial, 2026-07-03)

| Item | Result |
|---|---|
| S4 capture | Functionally works (sound reached both Computer and AirPlay speaker through Airfoil), **but with severe dropouts on macOS 26.5 in every configuration** — bare NodeApp output and direct macOS AirPlay are both clean, so the dropouts are Airfoil/ACE's (U7 risk confirmed; 44.1 kHz rate alignment did not help). Verdict: Airfoil unusable for production audio on Tahoe -> decision U8 |
| S5 dictionary | Verified against 5.12.6: commands are `connect to`/`disconnect from` (fixed in AirfoilBridge + tests); speaker `name/connected/volume` parse works incl. ru-locale comma decimals; **scripted app-source setter is broken** (silently yields missing value; system source sets fine); source resets on audio-config changes -> bridge must re-set source before reconnecting speakers. NSAppleScript hangs off-main-thread in CLI apps -> bridge runs osascript subprocesses (TCC attributes to NodeApp). Airfoil lists only regular, fully-launched apps as sources; apps inside dot-directories can't be launched by it (paramErr) |
| S6 latency | Order of magnitude: AirPlay path ≈ 2 s command-to-sound (both via Airfoil and direct CoreAudio AirPlay); local output < 100 ms. Consistent with spec ch. 14 expectations (large absolute offset, only the inter-home difference matters). Exact per-node numbers come from /offset_test at G3 |

Bonus findings hardened into NodeApp during S4-S6: full NSApplication lifecycle (NSApp.run + menu + activity assertion against App Nap), engine restart on output-device/config changes, GUI alert for config errors on Finder launches, production path `~/duet/NodeApp.app` (LaunchServices refuses launching apps from dot-directories).

**Phase 0 status: S1-S6 answered. G0 closes once the customer decides U8 (speaker delivery on macOS 26) — the only open spike question.**
