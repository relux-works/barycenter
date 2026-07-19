# Phase 1 independent protocol review — verdict

- Date: 2026-07-19
- Preparation task: `TASK-260715-3ffm3r` — Approve Phase 1 independent protocol review
- Original task: `TASK-260712-176b74` — Independent Phase 1 protocol and compatibility review
- Handoff authority: `acceptance/phase1/protocol-independent-review-handoff-v2.json` (contract `p1-independent-protocol-review-handoff.v2`)

## Verdict

**APPROVE.** No critical or high finding is open. `P1-PROTO-001` is confirmed
closed without regressing unknown-type forward compatibility. All 39 original
Phase 1 message mappings are preserved at the reviewed revision; the 20
additive stream/live-PTT messages and the optional `state.capture_quality`
object are capability- and version-isolated from Phase 1 clients. All required
suites were re-run by this reviewer and pass.

## Reviewer identity and independence

- Reviewer: **Claude Fable 5** (model id `claude-fable-5`), task-board spawn
  run `RUN-260719-a723c8`, session branch `review/task-260715-3ffm3r-fable5`.
- Authorization: task note dated 2026-07-19 — owner Ivan Oparin explicitly
  authorized a task-board-spawned Claude Fable 5 reviewer agent as the
  qualified non-implementing independent reviewer; no verdict was inferred
  from that authorization. This report is that reviewer's own verdict.
- Independence: this session implemented none of the reviewed protocol,
  scheduler, codec, golden or contract-test work. It was spawned 2026-07-19
  solely for this review, performed no code modification (read-only review;
  the only writes were board mutations and scratch files outside the repo),
  and is a separate execution chain from every session that produced the
  reviewed commits.

## Reviewed revision

- Baseline (accepted Phase 1 merge, PR #68 "fix(protocol): reject
  incompatible major versions"): `524eb78e2d768ade1628d9170654f5f9c9d06e4b`.
- Reviewed candidate: `191ae26325ba34d32c94358044635fb7a73651e2` (exact later
  main head per AC).
- Review executed at repository HEAD `9e9da975cb8d407b55ca99346ed9496b79b52c86`
  (= origin/main); verified `git log 191ae263..HEAD` touches **zero** of the
  seven authority paths (tracking merges only), so the reviewed surface at
  HEAD is byte-identical to the pinned candidate.

## Packet verification (reproduced independently)

- `python3 scripts/acceptance/validate_p1_protocol_review_handoff.py` → valid.
- Recomputed myself: `git diff --name-status/--numstat 524eb78..191ae263`
  over the 7 authority paths → SHA-256 `8820139…dedaa6` / `9486…6833`, 51
  paths, +4610/−25 — all exactly matching the machine packet.
- Golden inventory recomputed: 39 baseline → 59 candidate files; 0 baseline
  goldens removed; 38 byte-unchanged; only `protocol/golden/state.json`
  modified (blobs `68abe1b` → `1e8081b`); the 20 additions are exactly the 12
  `stream_*` + 8 `live_ptt_*` files listed in the packet.

## Evidence re-run by this reviewer (all pass)

- Required: coordinator `go test ./internal/protocol`; pulsar-win
  `go test ./wire`; node-app `swift test --filter ProtocolContractTests`
  (9/9 including `versionMismatchRejected`, `goldenDirComplete` = 59,
  `roundTripEveryGolden`).
- Beyond required: coordinator **full `go test -race -count=1 ./...`** — every
  package ok, including protocol, hub, session, store (450s), media, bot,
  resolver, presentation and the `cmd/duet-coordinator` command runtime
  (233s). pulsar-win **full `go test -race -count=1 ./...`** — all 4 packages
  ok. node-app **full `swift test`** — 308 tests in 52 suites pass.
- Toolchain note: the workstation's selected CommandLineTools cannot build
  the Swift test bundle (`no such module 'Testing'`, the gap already recorded
  in the wire contract doc). I replicated CI's pinned full-Xcode approach via
  `DEVELOPER_DIR=/Applications/Xcode.app`; no system state was changed.
- Hosted corroboration: CI run `29684355308` (workflow `ci`) succeeded at
  `b2b7ca4`, whose authority surface is identical to the candidate; PR #68 is
  MERGED with merge commit `524eb78`.

## Independent inspection performed

1. **39 original mappings re-sampled.** All 39 baseline golden types are
   present and registered in all three codecs (Go factory = 59 entries;
   Windows mirror; Swift decode switch + `expectedTypeCount = 59`), enforced
   by count-equality and per-file strict round-trip tests on each platform.
   Field-level check of the P1-critical fixtures (`prepare_media`,
   `play_media_at`, `cancel_media`, `presence_update`, `set_dnd`, `register`,
   `media_failed`, `play_voice`) against the frozen contract tables: exact,
   including conditional-field omission (omitted, never `null`), overlay vs
   interrupt field exclusivity, DND-conditional `muted_until_coord_ms`, and
   `media_failed` carrying only `stage`/`code` (no diagnostic leakage).
2. **Closed enum tables.** Delivery (`overlay|interrupt|after_current`),
   14-value target status with status↔reason cross-validation
   (`validTransmissionTargetReason`), downgrade reason vocabulary, playback /
   FSM / `ended.reason` / `error.code` vocabularies in `docs/protocol.md` —
   traced into typed payloads, store validators and the contract-document
   tests in `coordinator/internal/protocol`.
3. **P1-PROTO-001 closure.** The version guard runs before payload dispatch
   in all three decoders (`codec.go` decode() first check; Swift
   `ProtocolCodec.decode` guard before the type switch). Runtime precedence
   is uniform: mixed major ⇒ terminate/reconnect, unknown type ⇒ warn+ignore
   — hub read loop checks `env.V` before `KnownType`; `awaitRegister` decodes
   (version-first) **before** `h.lookup(token)`; Swift `classifyIncoming`
   maps `versionMismatch → .reconnectVersion → scheduleReconnect` and
   `unknownType → .ignoreUnknown`. All five regression legs read and
   re-executed: coordinator codec (strict+lenient), pre-auth registration
   (test fails if credential lookup is ever reached), established-connection
   close, Windows read-loop reconnect (real second dial asserted), Swift
   runtime classification. A v2 frame with an unknown type terminates rather
   than being silently ignored — the correct non-additive semantics.
4. **Forward compatibility preserved.** Lenient runtime decode tolerates
   unknown payload fields (strictness is test-only); unknown v1 types are
   ignored with a warn at all three runtimes; capability sets retain unknown
   sorted names for diagnostics while `Supports()` stays exact-match.
5. **Windows mirror.** `protocol.go`, `codec.go`, `live_ptt.go`,
   `capture_quality.go` verified byte-identical to the coordinator sources
   (gofmt-normalized) after the declared mirror header — by my own `cmp` and
   by `TestMirrorMatchesCoordinatorSource`.
6. **Additive-family isolation (20 new messages + capture_quality).** All new
   messages are v1 additive (no envelope major change). Production
   advertisement excludes them: pulsar-win `main.go` advertises only
   `seamless_adoption_v1` + the initialized clip client's earned set; macOS
   `PlayerCore.advertisedCapabilities` likewise. Live-PTT binary frames are
   capability-gated at the hub and both clients; `capture_quality` state is
   capability-rejected at the hub and withheld client-side when
   unadvertised; stream work requires persisted per-target capability
   booleans at HTTP resolution, and the `StreamGenerationGuard` state machine
   is mirrored three-way with identical test transcripts. Phase 1-era clients
   that receive a new type ignore it (spec 8.6 path above).
7. **State machine / timing / legacy evidence.** The clip scheduler
   (`store/transmission_scheduler.go`) carries the exact frozen constants —
   3000 ms prepare barrier, `lead = max(2·maxRTT+250, 500)`, +100 ms start
   window, 10 s max-RTT staleness — with a 16-test suite covering domain
   FIFO, equal-time ULID ties, partial readiness with no late autoplay,
   block/DND/offline/clock rechecks, delivery-expiry disarm, ack-timeout
   convergence and authenticated receipt binding. Resolution tests pin
   concurrent-idempotency single acceptance, bound single-use interrupt
   confirmation, stale-binding capability rejection, and whole-transmission
   overlay downgrade (`mandatory_target_missing_overlay_capability`);
   interrupt fallback requires explicit sender confirmation. Telegram legacy
   race tests pin atomic replacement of only not-started work, start-wins
   (`too_late`), concurrent callbacks producing one replacement, and fault
   rollback. All of these executed inside the passing race suites.

## Findings

- **Critical/high: none.** `P1-PROTO-001` (HIGH) is closed and re-reviewed as
  above; no new critical or high finding was identified.
- Informational (no action required for this signoff):
  1. Local CLT toolchain cannot build the Swift test bundle; already
     documented in the wire contract, CI pins full Xcode, and the suite runs
     locally under `DEVELOPER_DIR`. Not a protocol defect.
  2. Two capability vocabularies exist at different layers (wire
     `stream_track_v1` vs store-resolution `stream_variant_v1` etc.). Both
     are documented contracts; no runtime conflict, but future work should
     keep the mapping explicit. Wire `CapabilityStreamTrack` is currently
     unreferenced outside tests, consistent with the production-dark posture.

## Evidence boundary

Repository and hosted-CI checks only. No real-app playback, audible, physical
device, packaged-install or Store claim is made or implied; those remain in
manual epic `EPIC-260714-th54l3`. This approval grants no production
activation, Store submission or release authority
(`productionOrStoreAuthorityGranted` remains false).

## Consequences

- `TASK-260712-176b74` checklist item 1 (reviewer independence) may be
  checked and the original task accepted, per this task's AC.
- Strict execution may advance past this gate (next: `TASK-260712-1uz0za`).
