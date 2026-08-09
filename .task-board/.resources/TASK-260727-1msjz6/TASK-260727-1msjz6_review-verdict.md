# TASK-260727-1msjz6 — Reviewer Verdict (RUN-260727-32989d, reviewer/claude)

- Reviewed: 2026-07-28  |  Source HEAD: 0445f3f  |  Coordinator: https://barycenter.relux.works
- Reviewed artifacts: receipt v2, evidence.tgz v2 (both resume RUN-260727-3a8672), board notes, live gate re-runs.
- Constraint honored: read-only review, no code changed.

## VERDICT: BLOCKED (evidence-backed stop-the-line) — honest PARTIAL PASS

The tester's v2 verdict is **confirmed accurate and honest**. Rows 1, 2, 9 = PASS (first-hand, both
machines). Rows 3–6, 8 = BLOCKED on a concrete human-only external blocker. Row 7 = BLOCKED under the
AC-permitted media/second-online-member exception. The identity journey PASSED and the membership journey has
an honest, non-PASS-claiming verdict with one precise owner step — exactly what the scope authorizes when the
disconnected Windows console makes GUI interaction impossible.

## Independent verification performed (first-hand, read-only)

1. **Two distinct Barycenters / no pairing (row 2) — CONFIRMED.** Mac orbit 9 / actor 20 / recovery
   rec_6e2f…0098b1e vs Windows orbit 10 / actor 21 / recovery rec_6fcd…55eadc; all four token fingerprints
   differ; stores differ (macOS Keychain bundle v2 vs DPAPI credentials.v1.dpapi 662 B, blob 07DC6E09…).
   Coordinator /healthz orbit count 9→10 proves a NEW Barycenter was minted, NOT a same-orbit device join.
   No credential/recovery-file sharing; no cross-machine copy.
2. **No secret leakage — CONFIRMED.** Only four 64-hex strings exist across every artifact, and all four are
   binary/blob SHA-256 hashes (NodeApp, pulsar-win-amd64.exe, go-librespot.exe, DPAPI blob). No
   node_token/control_token/bearer/secret value is present; tokens appear only as 16-hex fingerprints.
3. **No device pairing / no production mutation — CONFIRMED.** Recon scripts contain only read verbs
   (SHA256 fingerprinting + DPAPI Unprotect); no invite/create/pair/POST/Set-/Remove-. The Windows credential
   is DPAPI-decrypted in-memory for PUBLIC fields only and the plaintext buffer is zeroed ([Array]::Clear).
4. **Gates green — CONFIRMED first-hand this review:**
   - `go vet ./internal/store/ ./cmd/duet-coordinator/` → exit 0.
   - `go test ./internal/store/ ./cmd/duet-coordinator/ -run Air` → **42 Air tests PASS, 0 FAIL**, incl.
     fanout invariants `TestAirRuntimeResolutionUsesOnlyCurrentMembersAndStableSnapshotKey` +
     `TestAirRegressionEightBarycentersTwentyPulsarsExactFanout`, and the single-use-invite invariant in
     `air_control_test.go` (consume → idempotent replay → third consume → `ErrAirInviteUnavailable`).
   - `swift test --filter Air` (node-app) → 8/8 PASS, exit 0.
   - `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c` (pulsar-win) → exit 0, 18,364,416 B (matches receipt).
5. **Anomaly A — CORROBORATED first-hand** from masked Windows log: credentials.v1.dpapi written 20:45:11Z,
   but the running process (started 20:44:37Z) stayed in the unpaired-shell tray loop
   (`windows shell message loop started`, no paired `orbit=` line); coordinator nodes_connected stayed 1.
6. **Autonomous path genuinely exhausted — CONFIRMED.** `PulsarProbe`/`winprobe` is a microphone/capture-only
   MSIX probe (its only added device capability is `microphone`); it is NOT an Air UI-automation harness. No
   approved Air UI harness exists, and non-interactive ssh session-0 cannot reach interactive desktop
   session-1 to drive the GUI. Forging rows via raw prod API + extracted control tokens was correctly refused
   (secret policy + no-forced-fit).

## Why BLOCKED (not done, not to-dev)

- **Not `done`:** AC requires the real Air create→invite→join→membership→restart→reuse journey (rows 3–6, 8)
  to actually PASS by observation. That did not happen. Accepting would be dishonest.
- **Not `to-dev`:** no autonomous producer rework advances rows 3–8 — the blocker is human-only physical GUI
  interaction on a Windows console-only machine plus the Anomaly A relaunch. Rerouting reproduces the same
  BLOCKED. No code defect in the delivered acceptance work.
- **Is stop-the-line `blocked`:** a concrete external/human-only blocker, with full evidence, exhausted
  attempts, the Anomaly A finding + deterministic remedy, a viable alternative, and one precise owner step.

## Exact human input needed (single owner step)

At the physical Windows console (interactive session, NOT ssh):
1. Relaunch Pulsar; verify coordinator `nodes_connected` becomes 2 (Anomaly A remedy). If it still starts
   unpaired despite credentials.v1.dpapi present, that is a client bug to file — recommend the app team
   confirm whether the in-shell onboarding→paired auto-transition is expected to fire (rhymes with
   BUG-260727-1hjfxi, done).
2. On Mac: create an Air + issue a member invite; hand only the one-time code out of band.
3. On Windows: Join Air → review → confirm.
4. Verify on both: exactly two members, expected active/saved state; restart both and re-verify; reuse the
   same invite (must fail safely); dissolve the Air and confirm cleanup.

Alternative that removes the owner dependency: provide an approved Air UI-automation harness (or a surviving
interactive Windows path) so rows 3–8 can be driven headlessly.

## Notes for the orchestrator
- Rows 10–11 (write tests / 80% coverage) correctly left unchecked: no product code changed and the journey
  invariants are already covered by the green Air suites. Authoring synthetic two-orbit/hardware-simulating
  tests would legitimize a forced fit; agreed.
- Anomaly A is worth a tracked follow-up bug for the app team (onboarded-but-not-connected in-process).
