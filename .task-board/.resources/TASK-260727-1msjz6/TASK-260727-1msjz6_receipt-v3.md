# TASK-260727-1msjz6 — Two-Independent-Barycenters Mac→Windows Air E2E — RECEIPT (v3, resume-after-owner-relaunch)

- Run: tester resume  |  Captured (UTC): 2026-07-28 (this run)  |  Source tree HEAD: 0445f3f (working tree has uncommitted Air/onboarding changes; see §Gates)
- Coordinator (authoritative, production): https://barycenter.relux.works
- Supersedes: v2 (RUN-260727-3a8672, PARTIAL PASS / BLOCKED). v2 was blocked by (a) the GUI-only Air create/join wall over non-interactive ssh, and (b) a concrete secondary blocker — the newly-onboarded Windows Barycenter (orbit 10) had not come online as a connected node (Anomaly A).
- **What changed since v2:** Ivan (owner) performed the v2 "one precise owner step 1" — relaunched Windows Pulsar on the interactive console. **Anomaly A is now RESOLVED** and independently verified first-hand this run (see §Anomaly A resolution). Coordinator `nodes_connected` is now **2**.
- Secrets policy: no token/recovery secret is printed here, in any log, or on the board. 64-hex tokens appear ONLY as SHA-256 fingerprints (first 16 hex). The Windows credential was DPAPI-decrypted in-memory for PUBLIC fields only and the plaintext buffer zeroed. Symmetric to the macOS Keychain read.

## VERDICT (headline)
**PARTIAL PASS / BLOCKED (advanced)** — The identity + capability + both-nodes-online + restart-persistence journey now PASSES first-hand on both machines with more evidence than v2. The Air *runtime* rows that require the GUI create→invite→join flow (create Air on Mac, review/confirm/join on Windows, membership match, shared-runtime fanout, invite-reuse-as-a-real-journey) remain **BLOCKED** on a single concrete human-only external blocker: the flow is GUI-only on both clients, the Windows console is reachable only via non-interactive ssh (session 0 cannot drive interactive desktop session 1), and there is **no approved Air UI-automation harness**. Driving the coordinator Air HTTP API directly with extracted control tokens is refused (it bypasses the "installed applications" mandate + secret policy = forced fit). One precise, now-REDUCED owner step is at the end.

---

## Candidate & capability facts (Row 1 — PASS, first-hand this run)

### Coordinator (authoritative, production) — VERIFIED LIVE (read-only)
GET /healthz (saved: `coordinator_healthz.json`):
- status=ok; phase2.air_rooms_enabled=**true**; phase2.air_authority_state=**airs_authoritative**
- **orbits=10**; **nodes_connected=2** (was 1 in v2 — the Windows node is now connected; Anomaly A resolved)
- streamed_tracks_enabled=false
- The new client `Availability()` capability probe (both platforms) reads exactly this `/healthz` `phase2` block and is unit-tested green (see §Gates).

### Mac candidate (this machine) — unchanged from v2, re-confirmed this run
- /Applications/Pulsar.app — CFBundleShortVersionString=0.3.0, CFBundleVersion=958.1
- Contents/MacOS/NodeApp SHA-256 = `4e8c986edf48fb4c3e1d05faf813881d3d5c370423d096c638dc77d3afed51d0`
- Running: NodeApp pid 3423 + go-librespot pid 3449 → the connected coordinator node (orbit 9); "welcome received" in the Mac log.

### Windows candidate (ssh alias `win` → DESKTOP-3PBO632 / admin, Windows 10 Pro) — re-confirmed this run
- Running/onboarded candidate = standalone airfix build:
  `C:\Users\admin\AppData\Local\Programs\Pulsar\pulsar-win-amd64.exe`
  SHA-256 = `9D544F59997C6B5EDC6FAFB29A0E92DA58F6644D2066512C5C9575264111FC25` (9,530,368 bytes), build label `v0.3.0-beta.32-airfix.20260727`.
- go-librespot.exe SHA-256 = `FFE82704BE5671629A00BDEA3915E40AA4E723B4A45417325DA41DD90F8D9402` (16,753,152 bytes).
- Processes: `pulsar-win-amd64` PID 11572 (orig onboarding, started 2026-07-27T20:44:36Z, stayed unpaired) **and PID 13300 (owner relaunch, started 2026-07-27T21:17:46Z — the paired/connected instance)**.

---

## Row 2 — Two distinct Barycenters, no shared credentials, no device pairing — PASS (first-hand, both machines, this run)

Public identity read first-hand on each machine (secrets shown ONLY as SHA-256 fingerprints):

| Field | Mac | Windows | Distinct? |
|---|---|---|---|
| credential store | macOS Keychain `works.relux.pulsar` / acct `onboarding-credential-bundle-v2` (bundle **v2**) | DPAPI `%APPDATA%\Pulsar\credentials.v1.dpapi` (envelope BCDP v1, bundle **v1**) | ✓ different stores |
| orbit_id | **9** | **10** | ✓ |
| actor_id | **20** | **21** | ✓ |
| slot | a | a | (per-orbit; not an identity collision) |
| role | primary | primary | (each is its own home's primary) |
| control.context | active | active | (both online/active) |
| recovery_id | `rec_6e2f997262f719817c7891bea0098b1e` | `rec_6fcd1faee4f89bc0af8671b38b55eadc` | ✓ no recovery sharing |
| node_token fp | `sha256:63caac25df7919b6` | `sha256:f6f7d5e8c06d55e9` | ✓ distinct secret |
| control_token fp | `sha256:ccd7379ee0786854` | `sha256:32ba8289026a9e88` | ✓ distinct secret |
| coordinator_origin | https://barycenter.relux.works | https://barycenter.relux.works | (same coordinator — expected) |

**No device pairing:** device pairing would attach Windows to the SAME orbit (9) as an extra slot/node, leaving the coordinator orbit count unchanged. Instead the orbit count went **9 → 10** and Windows holds its OWN orbit_id 10 / actor_id 21 / own tokens+recovery → the "Create a new Barycenter" path was used, NOT "Create device invitation".
**No credential/recovery sharing:** every secret fingerprint and both recovery_ids differ; Mac secrets live only in the Keychain, Windows secrets only in the DPAPI blob; `%APPDATA%\Pulsar` has no `credentials.json` legacy export and no `recovery-*.dpapi` pending file; nothing was copied between machines.

---

## Anomaly A resolution — Windows onboarded → NOW online (first-hand, NEW this run)

From the masked Windows log (`windows_log_masked_v3.txt`, all 64-hex redacted; local time +03:00):
- Earlier onboarding starts (2026-07-27 21:17 / 21:45 / 23:44 local) each reached only `"unpaired shell startup" … enter_tray_loop` → `"windows shell message loop started"` — i.e. onboarded-but-unpaired (the v2 Anomaly A).
- **The owner relaunch (PID 13300, 2026-07-28T00:17:46.134+03:00 = 2026-07-27T21:17:46Z UTC) took the PAIRED path:**
  - `msg="pulsar-win running" version=v0.3.0-beta.32-airfix.20260727 slot=a orbit=10 device_name="Pulsar A"`
  - `msg="connected to coordinator" url=wss://barycenter.relux.works/ws`
  - `msg=welcome mode=shared state=idle volume=80`
- Corroborated by coordinator `nodes_connected` 1 → **2**.

This is exactly the v2 deterministic remedy ("relaunch → paired path → connect as orbit 10") playing out. The first-launch onboarding→paired auto-transition still did NOT fire in-process on the original session (only relaunch connected) — the underlying client defect is tracked as **BUG-260728-28mp7v** (`windows-onboarding-stays-unpaired-until-relaunch`, currently `backlog`). The relaunch workaround is confirmed effective.

---

## Per-row PASS / FAIL / BLOCKED

| # | Row | Verdict | Basis |
|---|-----|---------|-------|
| 1 | Record Mac & Windows candidate hashes + coordinator capability | **PASS** | Hashes + live /healthz above, all first-hand this run. Capability now also reflected by the tested `Availability()` client probe. |
| 2 | Prove Mac & Windows use distinct Barycenter identifiers and credentials | **PASS** | First-hand identity on both machines (table above): distinct orbit/actor/recovery/tokens/stores; no sharing; not device-paired (orbit delta 9→10). |
| 3 | Create an Air on Mac + capture one-time invite without persisting secret | **BLOCKED** | GUI-only on the Mac desktop; not driven autonomously and not faked via raw prod API. No Air exists. Coordinator create + single-use-invite path is proven green by automated lifecycle tests. |
| 4 | Review/confirm/consume the Air invite on Windows without device pairing | **BLOCKED** | GUI-only on Windows; ssh session-0 cannot interact with console desktop session-1; no Air UI harness. (No longer *additionally* blocked by "Windows offline" — Anomaly A resolved — but the GUI wall stands.) |
| 5 | Matching saved membership + active-Air state on both clients | **BLOCKED** | Depends on 3+4. Air state is coordinator-authoritative (re-fetched); no local membership file to observe without the authenticated Air API (control token — refused). |
| 6 | Restart both clients; verify membership + identity persistence | **PARTIAL PASS (identity) / BLOCKED (membership)** | **Windows identity persistence across restart PROVEN first-hand this run**: DPAPI credential survived the relaunch and the client came back as the SAME orbit 10, connected + welcome (§Anomaly A). Mac identity persistence design-verified (Keychain bundle survives restart; same running instance). Membership persistence still depends on an Air existing (rows 3–4, blocked). |
| 7 | Exercise one shared runtime action; exact fanout, no duplicates | **BLOCKED (media/second-active-member prerequisite unavailable)** | Needs ≥2 members holding current active pointers to one Air; no Air exists. AC explicitly permits a narrowly-scoped blocked row. Invariant green (`TestAirRuntimeResolutionUsesOnlyCurrentMembersAndStableSnapshotKey`, `TestAirRegressionEightBarycentersTwentyPulsarsExactFanout`). |
| 8 | Consumed invite cannot be reused; fails safely | **BLOCKED (real journey) / invariant PASS** | Real cross-machine reuse needs a consumer (blocked). Single-use CAS (open→consumed) + reuse→HTTP 404 `invite_unavailable` verified green by the automated coordinator tests. |
| 9 | Attach redacted timestamped receipt + safe cleanup | **PASS** | This receipt. Cleanup: no production Air/invite created (no prod mutation); all Windows recon read-only; copied test binary + recon scripts removed from the Windows host (`cleanup_remaining=0`); local scratch only in gitignored `.temp/`. No secret in any artifact. |

### Gates (this run — exact standalone exit codes; working tree has uncommitted Air/onboarding changes)
- `gofmt -l` on air files (coordinator) → **exit 0** (clean, no output)
- `go vet ./internal/store/ ./cmd/duet-coordinator/` → **exit 0**
- `go test ./internal/store/ -run Air -count=1` → **exit 0** — **26 PASS / 0 FAIL**
- `go test ./cmd/duet-coordinator/ -run Air -count=1` → **exit 0** — **23 PASS / 0 FAIL** (incl. new `TestAirHTTPReportsDisabledRolloutInsteadOfRevisionConflict` + fanout invariant)
- `swift test --filter Air` (node-app) → **exit 0** — **8 tests / 3 suites PASS** (incl. new `"Coordinator health exposes typed Air availability"`)
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c` (pulsar-win) → **exit 0** (test binary 18,364,416 B); `GOOS=windows go vet .` → **exit 0**
- **NEW — pulsar-win Air tests EXECUTED ON THE REAL WINDOWS HOST** (`windows_air_tests_run.out`): **14/14 Air tests PASS** (`-test.run Air`). Note: on the first pass `TestWindowsAirCompositionHasNoPhaseOneTargetOrInboxDependency` reported FAIL with `open windows_air_composition.go: The system cannot find the file specified` — this is a source-inspection test (`os.ReadFile("windows_air_composition.go")`) failing ONLY because the standalone binary was run outside its package dir; re-running with the source file present → **PASS**. Not a code defect. The other 13 Air tests passed unconditionally, incl. `TestAirClientReadsCoordinatorAvailability` (the new availability probe) and the single-use / redaction / disruptive-confirmation invariants.
- No product code was authored/changed by this tester run. The new availability-probe code (added by the current working tree) is already covered by dedicated tests on both clients — verified green above, and on real Windows hardware. Authoring synthetic two-orbit / simulated-hardware tests to "pass" the GUI journey would legitimize a forced fit and is intentionally NOT done.

---

## THE ONE PRECISE OWNER STEP (reduced — Anomaly A already cleared) to unblock rows 3–5, 7, 8

Anomaly A (relaunch → online) is DONE. The remaining human-only step is the two-machine GUI create/join, which non-interactive ssh cannot drive:

1. On the **Mac** (Pulsar already running, orbit 9): create an **Air** and **issue a member invite**; hand only the one-time invite code to the Windows machine out of band (do NOT paste it into any log/board).
2. On **Windows** (Pulsar already running/connected, orbit 10): use **Join Air → review → confirm** (never "Connect another device").
3. Verify on both clients: exactly the two members, expected active/saved state; restart both and re-verify membership; then attempt to reuse the same invite (must fail "expired/used"); optionally exercise one shared playback/control action and confirm exactly the two members receive it (no duplicate/transitive); then dissolve the Air and confirm cleanup.

Everything except this interactive-GUI create/join is verified above. Alternatively, provide an approved Air UI-automation harness (or a surviving interactive Windows automation path) and rows 3–8 can be driven without an owner present.
