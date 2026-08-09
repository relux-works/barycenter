# TASK-260727-1msjz6 — Two-Independent-Barycenters Mac→Windows Air E2E — RECEIPT (v2, resume)

- Run: RUN-260727-3a8672 (tester)  |  Captured (UTC): 2026-07-27T20:30Z–21:05Z
- Source tree HEAD: 0445f3f  |  Coordinator: https://barycenter.relux.works
- Supersedes: v1 (RUN-260727-67f1d0), which was headline-BLOCKED because Windows had no Barycenter.
- **Owner input (2026-07-27T20:45Z):** Ivan launched Pulsar in the interactive Windows console session
  and completed **Create a new Barycenter**. This run resumes from identity verification and independently
  observes each row; no downstream Air row is assumed to pass without first-hand evidence.
- Secrets policy: no token / recovery secret is printed here, in logs, or on the board. 64-hex tokens are
  referenced only by SHA-256 fingerprint (first 16 hex). The Windows credential was decrypted in-memory,
  read for PUBLIC fields only, and the plaintext was zeroed; no secret left the machine. This is symmetric
  to the macOS Keychain read.

## VERDICT (headline)
**PARTIAL PASS / BLOCKED** — The core unblock succeeded: **two distinct, independent Barycenters now
provably exist on the two machines with no shared credentials or recovery material, created WITHOUT device
pairing** (row 2 = PASS, first-hand on both machines). The remaining Air-runtime rows (create Air on Mac,
review/confirm/join on Windows, membership match, restart persistence, shared fanout, invite-reuse) stay
**BLOCKED**: (a) they are GUI-only and cannot be driven from the non-interactive ssh (session 0 cannot
interact with the interactive desktop session 1), and there is no approved Air UI-automation harness; and
(b) a concrete new blocker — the newly-onboarded Windows Barycenter (orbit 10) **is not yet an online,
connected coordinator node** (the running process is still in the unpaired-shell tray loop; see Anomaly A).
Faking these rows via raw production API calls with extracted control tokens is refused (secret policy +
no-forced-fit). One precise owner step is at the end.

---

## Candidate & capability facts (row 1 — PASS, first-hand)

### Coordinator (authoritative, production) — VERIFIED LIVE (read-only)
- GET /healthz: status=ok; phase2.air_rooms_enabled=**true**; phase2.air_authority_state=**airs_authoritative**;
  **orbits=10** (was 9 in v1 — a NEW Barycenter was minted by the Windows onboarding); nodes_connected=1
  (only the Mac node is connected; the Windows node is not — see Anomaly A). streamed_tracks_enabled=false.
- Blocking work items are now resolved: TASK-260727-1f2cyl (authority rollout) = **done**;
  BUG-260727-1hjfxi (air-creation stale-revision / confused onboarding) = **done**.
- (version string git-3565c1e is a known-stale build label per TASK-260727-1f2cyl, which is done.)

### Mac candidate (this machine) — unchanged from v1
- /Applications/Pulsar.app — CFBundleShortVersionString=0.3.0, CFBundleVersion=958.1
- Contents/MacOS/NodeApp SHA-256 = `4e8c986edf48fb4c3e1d05faf813881d3d5c370423d096c638dc77d3afed51d0`
- Running: NodeApp pid 3423 + go-librespot pid 3449 → the single connected coordinator node (orbit 9).

### Windows candidate (ssh alias `win` → DESKTOP-3PBO632 / admin, Windows 10 Pro 19045) — CORRECTED
- The **running/onboarded candidate is the standalone airfix build**, NOT the MSIX:
  `C:\Users\admin\AppData\Local\Programs\Pulsar\pulsar-win-amd64.exe`
  SHA-256 = `9D544F59997C6B5EDC6FAFB29A0E92DA58F6644D2066512C5C9575264111FC25` (9,530,368 bytes)
  build label `v0.3.0-beta.32-airfix.20260727`; go-librespot.exe SHA-256 =
  `FFE82704BE5671629A00BDEA3915E40AA4E723B4A45417325DA41DD90F8D9402` (16,753,152 bytes).
- Running: `pulsar-win-amd64` PID 11572 since 2026-07-27T20:44:36Z; interactive console session 1 (admin) active.
- MSIX `ReluxWorksLLC.PulsarBarycenter_0.3.32.0_x64__q036g2bzd7ngc` is still installed but is NOT the running
  candidate. (v1 looked for identity in the MSIX AppContainer LocalState and found none; the airfix binary
  stores identity under `%APPDATA%\Pulsar` — where it was found this run. This corrects v1's "never onboarded".)

---

## Row 2 — Two distinct Barycenters, no shared credentials, no device pairing — PASS (first-hand, both machines)

Public identity read first-hand on each machine (secrets shown ONLY as SHA-256 fingerprints):

| Field | Mac | Windows | Distinct? |
|---|---|---|---|
| credential store | macOS Keychain `works.relux.pulsar` / acct `onboarding-credential-bundle-v2` (bundle v2) | DPAPI file `%APPDATA%\Pulsar\credentials.v1.dpapi` 662 B, blob SHA-256 `07DC6E09…` (bundle v1) | ✓ different stores |
| orbit_id | **9** | **10** | ✓ |
| actor_id | **20** | **21** | ✓ |
| slot | a | a | (per-orbit; not an identity collision) |
| role | primary | primary | (each is its own home's primary) |
| recovery_id | `rec_6e2f997262f719817c7891bea0098b1e` | `rec_6fcd1faee4f89bc0af8671b38b55eadc` | ✓ no recovery sharing |
| node_token fp | `sha256:63caac25df7919b6` | `sha256:f6f7d5e8c06d55e9` | ✓ distinct secret |
| control_token fp | `sha256:ccd7379ee0786854` | `sha256:32ba8289026a9e88` | ✓ distinct secret |
| coordinator_origin | https://barycenter.relux.works | https://barycenter.relux.works | (same coordinator — expected) |

**Proof of no device pairing:** device pairing would attach Windows to the SAME orbit (9) as an additional
slot/node, leaving the coordinator orbit count unchanged. Instead the coordinator orbit count went **9 → 10**
and Windows holds its **own orbit_id 10 with its own actor_id 21 and its own tokens/recovery** — i.e. the
"Create a new Barycenter" path was used, and the forbidden "Create device invitation" path was NOT.

**Proof of no credential/recovery sharing:** every secret fingerprint and the recovery_id differ; Mac secrets
live only in the macOS Keychain, Windows secrets only in the DPAPI blob; `%APPDATA%\Pulsar` has no
`credentials.json` legacy export and no `recovery-*.dpapi` pending file; nothing was copied between machines.

---

## Per-row PASS / FAIL / BLOCKED

| # | Row | Verdict | Basis |
|---|-----|---------|-------|
| 1 | Record Mac & Windows candidate hashes + coordinator capability | **PASS** | Hashes + /healthz above, all first-hand this run. |
| 2 | Prove Mac & Windows use distinct Barycenter identifiers and credentials | **PASS** | First-hand identity on both machines (table above): distinct orbit/actor/recovery/tokens/stores; no sharing; not device-paired (orbit delta 9→10). |
| 3 | Create an Air on Mac + capture one-time invite without persisting secret | **BLOCKED** | GUI-only on the Mac desktop; not driven autonomously and not faked via raw prod API. No Air exists yet (Mac log shows no recent Air activity; nodes_connected=1). Coordinator create+single-use-invite path is proven green by the automated lifecycle test. |
| 4 | Review/confirm/consume the Air invite on Windows without device pairing | **BLOCKED** | GUI-only on Windows; session-0 ssh cannot interact with the console desktop; no Air UI harness. Additionally blocked by Anomaly A (Windows node not online). |
| 5 | Matching saved membership + active-Air state on both clients | **BLOCKED** | Depends on 3+4. Air state is coordinator-authoritative (re-fetched), so there is no local membership file to observe without the authenticated Air API (would require a control token — refused). |
| 6 | Restart both clients; verify membership + identity persistence | **BLOCKED** | Depends on 4. Mac identity persistence is design-verified (Keychain bundle survives restart). Windows identity now persists on disk (DPAPI blob present) but has not yet produced a connected node even once (Anomaly A). |
| 7 | Exercise one shared runtime action; exact fanout, no duplicates | **BLOCKED (media / second online member prerequisite unavailable)** | Needs ≥2 online members holding current active pointers; Windows is not an online node and no Air exists. AC explicitly permits a narrowly-scoped blocked row. Invariant itself is green (`TestAirRuntimeResolutionUsesOnlyCurrentMembersAndStableSnapshotKey`). |
| 8 | Consumed invite cannot be reused; fails safely | **BLOCKED (real journey) / invariant PASS** | Real cross-machine reuse needs a consumer (blocked). Single-use CAS (open→consumed) + reuse→HTTP 404 `invite_unavailable` is verified green by the automated coordinator tests. |
| 9 | Attach redacted timestamped receipt + safe cleanup | **PASS** | This receipt. Cleanup: no production Air/invite created (no prod mutation); all Windows recon read-only; the Windows credential was decrypted in-memory for PUBLIC fields only and the plaintext zeroed; local scratch only in gitignored `.temp/`. No secret in any artifact (scanned). |

### Automated gate evidence (this run — exact standalone exit codes)
- `gofmt -l` on air files → **exit 0** (clean, no output)
- `go vet ./internal/store/ ./cmd/duet-coordinator/` → **exit 0**
- `go test ./internal/store/ -run Air -count=1` → **exit 0** (ok 2.314s)
- `go test ./cmd/duet-coordinator/ -run Air -count=1` → **exit 0** (ok 1.702s)
- `swift test --filter Air` (node-app) → **exit 0** — 8 tests / 3 suites pass, incl.
  "Every lifecycle action uses the common authenticated Air API" and
  "Coordinator health exposes typed Air availability".
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c` (pulsar-win) → **exit 0** (test binary 18,364,416 B).
- No product code changed this run; no new tests authored — the journey's coordinator/client invariants are
  already covered by the existing Air suites (green above). Authoring synthetic two-orbit / simulated-hardware
  tests would legitimize a forced fit and is intentionally not done (rows 10–11 left unchecked accordingly).

---

## Anomaly A — Windows onboarded but not online as a connected node (finding)

Observed, first-hand:
- `credentials.v1.dpapi` written 2026-07-27T20:45:11Z (orbit 10 / actor 21) — onboarding persisted the identity.
- The running process (PID 11572, started 20:44:36Z) is still in the **unpaired-shell tray loop**: last log
  line is `windows shell message loop started`; there is NO `unpaired shell stopped` and NO paired
  `pulsar-win running orbit=…` line. Coordinator `nodes_connected` stayed 1 (Mac only).
- Code path (`pulsar-win/unpaired_shell_windows.go`): the unpaired shell only transitions to the connected
  node when onboarding fires `didPair.Store(true)` + `requestTrayLoopExit()` and the tray loop returns to
  `main.run()`'s paired path. That transition did not occur, so orbit 10 never came online this session.

This rhymes with BUG-260727-1hjfxi ("air-creation stale-revision / confused onboarding"), which is marked
done — but the currently-installed airfix binary still exhibits "identity written, node did not come online
in-process." **Deterministic remedy: relaunch Windows Pulsar** — on restart, credentials are present, so
`main.run()` takes the paired path and connects as orbit 10. Worth the app team's confirmation of whether the
in-shell auto-transition is expected to fire here.

---

## THE ONE PRECISE OWNER STEP (to unblock rows 3–8)

At the physical Windows machine (DESKTOP-3PBO632), interactive console session (NOT ssh):
1. **Quit and relaunch Pulsar** (Desktop\Pulsar.lnk → the airfix binary). It should now start paired and
   connect as orbit 10 (verify: coordinator `nodes_connected` becomes 2). If it still starts unpaired despite
   `credentials.v1.dpapi` being present, that is a client bug to file, not an owner error.
2. On the **Mac**, in Pulsar create an **Air** and **issue a member invite**; hand only the one-time invite
   code to the Windows machine out of band.
3. On **Windows**, use **Join Air → review → confirm**.
4. Verify on both clients: exactly the two members, expected active/saved state; restart both and re-verify;
   attempt to reuse the same invite (must fail "expired/used"); then dissolve the Air and confirm cleanup.

Everything except this human interactive-GUI create/join is verified above. Alternatively provide an approved
Air UI-automation harness (or a surviving interactive Windows path) and the remaining rows can be driven
without an owner present.
