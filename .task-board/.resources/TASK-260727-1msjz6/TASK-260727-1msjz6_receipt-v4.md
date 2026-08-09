# TASK-260727-1msjz6 — Two-Independent-Barycenters Mac→Windows Air E2E — RECEIPT (v4, resume-after-owner-join)

- Run: tester resume RUN-260727-b63b90  |  Captured (UTC): 2026-07-27T22:33Z–22:53Z (local +04:00 → 02:33–02:53 on 2026-07-28)
- Coordinator (authoritative, production): https://barycenter.relux.works  |  Source tree HEAD: 0445f3f (uncommitted Air/onboarding changes present)
- Supersedes v3 (RUN-260727-331f4b). Resumes after the owner (Ivan) reported: Windows online (relaunch), Mac GUI **created Air "hhome-test" + issued one member invite** ("код есть"), invite scp'd to Windows Desktop, and Windows **Air Join → Review → Confirm** ("вошел").
- Secret policy honored: NO invite code / token / recovery secret is in this receipt, any log, any bundled artifact, or the board. The four full-window Air screenshots that transiently showed the live invite code were **deleted**; only a membership-card crop (code excluded) is retained. 64-hex values referenced only as public hashes/fingerprints.

## VERDICT (headline)
**BLOCKED (evidence-backed stop-the-line) — the reported Windows join did NOT produce a `joined` membership, and the ROOT CAUSE is now identified.** Read-only, first-hand on the Mac (the authoritative Air owner view, re-fetched via explicit "Refresh Airs"), the test Air **"hhome-test" has exactly 1/8 members** (the Mac owner, orbit 9), **0 active**, **Room state Parked**. Windows (orbit 10) is **not** a joined member despite "вошел".

**ROOT CAUSE (new finding, first-hand):** The Windows client is in a **WebSocket reconnect storm** and there are **three concurrent `pulsar-win-amd64` processes** competing for the single orbit-10 identity. The node flaps `connected → welcome → close 1006 (abnormal closure) → reconnect` every ~2 s continuously (captured live at 22:52Z). A node that never holds a stable authenticated session cannot commit / stabilise an Air Review→Confirm into a `joined` membership. This is the same failure *family* as the earlier Anomaly A (onboarded-but-not-connected) and BUG-260728-28mp7v, one layer up: onboarded + intermittently-connected-but-flapping due to multi-instance contention.

This is not autonomously recoverable: the Air Join/Confirm flow is GUI-only on the Windows console, and non-interactive ssh (session 0) cannot drive the interactive desktop (session 1); driving the coordinator Air API directly with an extracted control token is refused (installed-apps mandate + secret policy = forced fit). The remedy is a precise, now-much-sharper owner step (below).

---

## What was verified read-only this run

### Coordinator capability + connectivity (Row 1) — PASS, first-hand
`GET /healthz` (public, saved `coordinator_healthz.json`): `status=degraded` (media processing unavailable — unrelated to Air), `orbits=10`, `nodes_connected=2`, `phase2.air_rooms_enabled=true`, `phase2.air_authority_state=airs_authoritative`.
Note: `nodes_connected=2` is a point-in-time snapshot; the Windows node is flapping (see Root cause), so it is not *stably* 2.

### Two distinct Barycenters, no pairing (Row 2) — PASS
`orbits=10` unchanged ⇒ no new orbit minted and no device-pairing this run. Prior first-hand identity (v2/v3, reviewer-CONFIRMED): Mac orbit 9 / actor 20 / macOS Keychain bundle v2 vs Windows orbit 10 / actor 21 / DPAPI `credentials.v1.dpapi` (blob 07DC6E09…); distinct recovery ids and all four token fingerprints; no credential/recovery sharing.

### Air creation on Mac (Row 3) — OBSERVED (create side), first-hand
Mac Pulsar "Airs" view shows Air **"hhome-test"**, **Your Air role: Owner**, effective invite policy **"Owner/admin primary"**. The Mac issues **one-time invites** (15-min TTL, `AirInviteTTL = 15*time.Minute`; single-use; UI states "The code is sent only to the coordinator and is never stored in this app"). The create+invite capability is real and observed; the invite-secret-not-persisted property holds by design (client keeps it in memory only — corroborated by source: no on-disk invite persistence).

### Cross-machine JOIN + membership (Rows 4, 5) — FAIL / not achieved
The Mac owner view (`GET /v1/airs` + `/v1/airs/{id}`, re-fetched with explicit Refresh) reports **member_count = 1**. Coordinator `member_count` counts only `status='joined'` (`air_control.go:340`); `pending_confirmation` members are excluded. Therefore Windows is **not joined** — it is either at no-membership or stuck `pending_confirmation`; the owner-side view cannot distinguish these (no pending-member count is exposed to the owner). Evidence: `mac_air_membership_1of8_redacted.png` (1/8 members, 0 active, Parked, Owner). The AC-required 2-member shared Air **does not exist**.

### Join/confirm flow (for a precise remedy)
Both steps are on the **joiner (Windows) side**: `ConsumeAuthorizedAirInvite` (Review → creates a `pending_confirmation` membership + returns preview) then `ConfirmAuthorizedAirJoin` (Confirm → `joined`; requires `currentPrimary`, i.e. the Windows barycenter's own primary). The Mac owner does not participate in the confirm. So a stalled/flapping Windows node breaks exactly this hand-off.

### Restart / identity persistence (Row 6) — identity prior-PASS; membership N/A
Windows DPAPI identity persistence across restart was proven first-hand in v3; Mac identity persists (running instance, Keychain). Membership persistence is N/A (no membership exists).

### Shared runtime fanout (Row 7) — BLOCKED (AC-permitted)
No 2-member active Air exists, so no fanout to observe. AC explicitly permits a narrowly-scoped blocked row here. Fanout invariants remain green in the coordinator suite.

### Invite reuse fails safely (Row 8) — invariant PASS; real journey not reachable
Single-use CAS (open→consumed) + reuse → `ErrAirInviteUnavailable` → HTTP 404 `invite_unavailable` (`air_http.go:596`) is test-proven. A real cross-machine reuse test needs a successful consume→join first, which did not happen. **The Windows invite-code file was NOT deleted** (per owner instruction: keep it until the reuse-fails check is completed; that check is not reachable until a join actually lands).

### Receipt + safe cleanup (Row 9) — PASS
This receipt. No production Air/invite was created or mutated by this tester run — every coordinator/client interaction was a read (Mac `list()`/`detail()` GET refresh is read-only, confirmed at `MacAirAppComposition.swift:237`). All Windows access read-only. Code-bearing screenshots deleted; no secret in any artifact (scanned). Local scratch only under gitignored `.temp/`.

---

## Root cause evidence (masked, `windows_client_state_masked.txt`, captured 22:52:46Z live)
Three running instances: `pulsar-win-amd64` PID **7200** (start 01:29:05 +03:00), **11572** (23:44:36 — original onboarding, historically stayed unpaired), **13300** (00:17:46 — the v3 paired instance).
Continuous, current storm (every ~2 s):
```
level=INFO  msg="connected to coordinator" url=wss://barycenter.relux.works/ws
level=INFO  msg=welcome mode=shared state=idle volume=80
level=WARN  msg="ws receive failed, reconnecting" err="websocket: close 1006 (abnormal closure): unexpected EOF"
level=WARN  msg="librespot exited" err="exit status 1"
```
Interpretation: multiple instances share the single orbit-10 credential; the coordinator evicts the duplicate node connection → each reconnects → flap. The Air Review→Confirm cannot commit/stabilise while the node never holds a durable session.

---

## THE ONE PRECISE OWNER STEP (sharper than prior runs)
On the **Windows console** (interactive session, NOT ssh):
1. **Close ALL Pulsar instances** — Task Manager → End task on every `pulsar-win-amd64` (there are 3: PIDs 7200 / 11572 / 13300) and any tray icons.
2. **Launch exactly ONE** Pulsar. Wait ~30 s and confirm it is **stably connected** — the log should show a single `connected to coordinator` + `welcome` and then **no repeating** `ws receive failed … close 1006`. (Coordinator `/healthz nodes_connected` should sit steadily at 2.)
3. Only then, on Windows: **Airs → paste the one-time invite code → Review → Confirm** (never "Connect another device").
4. **Verify it actually landed:** on the Mac "Airs" card, "hhome-test" must flip from **1/8 → 2/8 members**. If it stays 1/8 (or Windows shows "Waiting for this barycenter's primary confirmation"), the Confirm didn't commit → press Confirm again / relaunch the single instance and retry.
5. After a confirmed 2/8: restart both and re-verify; attempt to reuse the same invite (must fail "used/expired"); then delete the Windows invite-code file; dissolve the Air and confirm cleanup.

Alternative that removes the owner dependency: an approved Air UI-automation harness (or a surviving interactive Windows automation path) + a client fix that prevents multiple concurrent instances from contending for one orbit identity.

## Recommendation for the orchestrator
File/track a client bug: **multiple `pulsar-win` instances are not prevented (no single-instance lock) → duplicate orbit-10 WS connections → close-1006 reconnect storm → Air join cannot stabilise.** Likely same family as BUG-260728-28mp7v (windows-onboarding/pairing state transitions). Rows 10–11 (write tests / 80% coverage) intentionally left unchecked: no product code was changed and the Air journey invariants are already covered by the green coordinator/client Air suites; authoring synthetic two-orbit/hardware-simulating tests would legitimize a forced fit.
