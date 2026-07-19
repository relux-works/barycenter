# TASK-260715-s838ym — Independent Phase 1 realtime-audio review verdict

- Date: 2026-07-19
- Decision: **APPROVE** — engineering scope only
- Approves: `TASK-260712-1uz0za` (p1-independent-realtime-audio-review) for
  repository-verifiable engineering evidence exclusively

## Reviewer identity and independence

- Reviewer: Claude Fable 5 (`claude-fable-5`), task-board tracked reviewer
  spawn run `RUN-260719-3e4ad6` on branch `review/task-260715-s838ym-fable5`.
- Owner authorization: the task notes record Ivan Oparin's approved default
  permitting a non-implementing task-board Claude Fable 5 reviewer that issues
  its own verdict.
- Independence: this session implemented none of the reviewed audio paths. The
  audited implementation and the corrective commit `805337d` (2026-07-15) were
  produced by earlier inline execution sessions; this review session began
  2026-07-19 and made zero modifications outside `.task-board` tracking
  (verified via `git status` before verdict: no source, test, or document
  changes by this session other than the acceptance tracking below).
- The frozen inline audit (`docs/analysis/p1-independent-realtime-audio-technical-audit.md`)
  explicitly disclaims reviewer independence; it was consumed as a technical
  packet only and not reused as signoff.

## Reviewed revision

- Reviewed head: `11b51320d7e8f020b16a1b779c815ad9771a2565` = `origin/main`
  head at review time (a later exact main head, as permitted by the AC).
- Contains audit merge `5aedd6817bece741b76408135271a5fb8da40a83` (PR #70,
  exactly one corrective commit `805337d0d572f6e45b90fc76120af29f21be89e3`
  plus merge) over frozen review base
  `aed5d7e5225aca0d4d5b0ad8347cfd500f6c0dac` (PR #69).
- Working tree at review matched the reviewed head bit-for-bit for all code
  paths (only pre-existing `.task-board` scope-reconciliation edits, which
  match the owner note dated 2026-07-19 on this task, were present and ride
  along in the acceptance commit).

## Method

1. Re-derived the reviewable invariants from
   `docs/render-safe-mixer-contract.md` (immutable control carrier, lifecycle
   ownership, render boundary, frozen gain order
   `limiter(main*duck + overlay/clip + cues) * master`, `-1 dBFS` local
   ceiling, pre-duck/anchor/resume/cancellation semantics, telemetry privacy)
   and checked the audit packet's claims against code at the reviewed head.
2. Inspected the full PR #70 corrective diff and each closed HIGH finding.
3. Inspected both render boundaries at the reviewed head, including every
   post-audit change to audited files (`5aedd68..11b5132`).
4. Reran deterministic, race, leak, memory and soak evidence locally where the
   toolchain allows, and consumed pinned hosted CI provenance at the exact
   reviewed head for the remainder.

## Render boundary inspection

**Windows (`pulsar-win/engine.go`).** `Render`, `renderMusic`, `mixOverlay`,
`mixInterrupt`, `mixLive`, `applyOverlayLimiter` and gain functions are
AST-guarded (`render_safety_test.go`) against goroutine creation,
`make/new/append`, `Lock/RLock/Wait/Sleep`; helpers (`rampValue`,
`beginInterruptRamp`, `dbAmplitude`) are pure math; completion posting is
non-blocking (`select`/`default` — audio timing wins);
`TestWindowsOverlayRenderAllocationsStayZero` measures zero allocations on the
active path at runtime. Branch limiter (live > interrupt > overlay ceiling) is
applied post-mix with master amplitude last, matching the frozen order.
`mixInterrupt` consumes the main ring only up to T (freeze, not silent
consumption), with raised-cosine pre-fade and deterministic late catch-up.

**macOS (`node-app/Sources/NodeCore/AudioEngine.swift`).** The marked render
callback performs one unconditional `ring.read`, uses fixed scratch and C11
acquire/release atomics (`RenderAtomicInt64`), consumes the SPSC gain-command
queue tail-side only, and contains no dispatch/lock/wait/allocation/I-O/sleep
tokens (`renderCallbackSourceSafety`). Graph order is pinned by
`overlayGraphGainOrder`: source → program mixer → DynamicsProcessor
(−1.1 dB threshold, 0.1 dB headroom = −1 dBFS ceiling) → mainMixer local
master.

## Closed HIGH findings — re-review

**P1-AUDIO-001 (Windows async failure/cancel-resume races) — closed,
verified.** `MediaClipMixer.Arm` now threads a typed `failed func(error)`;
`MediaClipClient.handleMixerFailure` emits `media_failed` with
schedule/playback stage and never `media_ended` (guarded against
terminal/cancelling phases). `finalizeInterrupt` serializes natural-end vs
cancel to one cached outcome (`interruptFinal` flag + `done` channel;
concurrent callers wait outside the lock and return the cached result).
`resume_main=false` routes to new `Player.AbandonInterrupt` — provider stays
paused, stale token released so later commands can re-own. Regressions:
`TestWindowsInterruptNaturalResumeFailureIsFailedNotEnded`,
`TestWindowsInterruptCancellationHonorsResumeMainFalse`,
`TestWindowsInterruptCancelDuringNaturalResumeAcknowledgesCachedResultOnce`,
`TestMediaClipAsyncMixerFailureIsTypedAndNeverReportedAsEnded` (before/after
start) — all rerun PASS under `-race` at the reviewed head.

**P1-AUDIO-002 (macOS reader/gain publication) — closed, verified.**
`readerActive` is `RenderAtomicInt64` (raw `UnsafeMutablePointer<Bool>`
removed); gain publishers serialize head update + slot write under
`gainCommandProducerLock` (NSLock) strictly outside the render boundary;
release-store of head after slot write pairs with the callback's acquire-load,
so the single render consumer stays lock-free and correctly ordered.
`renderControlPublicationSafety` requires the atomic, the producer lock, and
its absence inside the marked callback — PASS in CI at the reviewed head.

**P1-AUDIO-003 (macOS FIFO shutdown + heartbeat snapshot) — closed,
verified.** FIFO open is `O_RDONLY | O_NONBLOCK` with bounded idle polling
(EAGAIN → 3 ms sleep; EOF → close, 50 ms, reopen) and `readerActive` checks in
every loop, so `stopEngine()` is bounded with no writer; ring-full lossless
backpressure is preserved. Playback/anchor/speaker fields are private
queue-owned state and `statePayload` builds one `queue.sync` snapshot.
`playerStateSnapshotSafety` pins both — PASS in CI at the reviewed head.

No other critical or high engineering finding was identified in this review.

## Post-audit deltas to audited files (`5aedd68..11b5132`)

- `pulsar-win/engine.go` (+248) and `AudioEngine.swift` (+145): additive P2
  live-PTT source branches on both platforms. They follow the audited
  discipline — atomics-only control publication, render-owned envelope state,
  fixed rings/scratch, generation-epoch isolation against stale callbacks, the
  shared post-mix `-1 dBFS` limiter, and music ducking through the serialized
  gain path. `mixLive` was added to the Windows AST guard; new
  `livePTTRenderSourceSafety`, `streamTrackRenderSourceSafety`, jitter/capture
  source guards are green in CI.
- `PlayerCore.swift` (+42): additions run entirely inside `queue.sync`;
  unadvertised `stream_track_v1`/`live_ptt_v1` commands are rejected by both
  clients (`player.go` +6 mirrors this).
- Core audited P1 files unchanged post-audit: `media_clip.go`,
  `overlay_mixer.go`, `interrupt_player.go`, `MacOverlayMediaClipMixer.swift`,
  `MediaClipClient.swift`.

## Rerun evidence at the reviewed head

Local (this session, darwin/amd64, go1.26.0):

- Full portable Windows suite under the race detector:
  `go test -race -count=1 ./...` — **all packages ok** (`pulsar-win` 20.4 s,
  probe/winprobe/wire ok).
- Focused evidence set rerun verbose under `-race` — **11/11 PASS**: 100
  sequential overlays without growth or deadlock (soak/leak), maximum P1 clip
  one bounded decoded buffer < 64 MiB (memory), overlay render allocations
  stay zero (leak/realtime), render-boundary AST guard, freeze-at-T,
  audible-anchor resume with fade-in, stale-token invalidation, and the four
  P1-AUDIO-001 regressions.

Local Swift limitation (disclosed): the entire NodeCore test suite is
swift-testing based and this machine's toolchain lacks the `Testing` module
("no such module 'Testing'"), so `swift test` cannot build locally. The
project's designated authoritative Swift gate is the pinned hosted CI
full-Xcode job (`.github/workflows/ci.yml`, `run_automated.py --suite swift
--require-clean` = `xcrun swift test` on macos-15).

Hosted CI consumed at the exact reviewed head `11b5132` (run `29689344361`,
push to main, 2026-07-19): all four jobs **success** (`node-core`,
`pulsar-win` = plain + `-race` + Windows cross-builds, `coordinator`,
`pulsar-win-packaged-probe`). Downloaded `phase1-acceptance-swift` provenance
manifest: `suite=swift`, `status=pass`, `git.head=11b5132…`, `dirty=false`,
`scope=repository-automated-only`, `manualEvidence=not-run`; log records
**"Test run with 308 tests in 52 suites passed"** (audit-time count was
218/35; growth is new P2/P3 suites), including
`MacOverlayMediaClipMixerTests` and `MediaClipClientTests` suites, the
100-overlay and maximum-buffer cases, the interrupt cancel/resume set, and all
eight render-safety source tests. CI was also green at audit merge `5aedd68`
(run `29401813802`).

## Manual evidence boundary — preserved

This approval covers repository-verifiable engineering evidence only. Manual
A3/A4 real-app evidence, audible quality (clipping, pumping, route noise),
packaged-app behavior, real provider drift, and physical 200 ms / 500 ms
timing remain **open** and owned exclusively by `TASK-260712-2hodti`
(status: backlog) in `EPIC-260714-th54l3`. No such claim is made or may be
inferred from this verdict; the CI provenance itself records
`manualEvidence=not-run`. Checklist item 4 on `TASK-260715-s838ym` remains
intentionally unchecked per the owner's 2026-07-19 scope reconciliation.
Phase 1 root acceptance and Store submission remain withheld.

## Minor observations (non-blocking)

1. macOS live-gain publication writes three atomics (target bits, ramp frames,
   generation); a consumer could in principle observe a torn triple across two
   serialized producer updates for at most one callback, with a valid clamped
   target — bounded, inaudible-by-construction transient; P2 scope; no P1
   contract violation.
2. The Windows AST guard checks direct calls in the named render functions
   only; transitive coverage is provided by the runtime zero-allocation
   measurement and the race suite. Consistent with the audit's claims; no gap
   found in practice (helpers verified pure by inspection).

## Verdict routing

- `TASK-260712-1uz0za` → accepted (`done`) for engineering scope only.
- `TASK-260715-s838ym` → `done` with this verdict resource.
- Manual scope continues in `TASK-260712-2hodti`; later independent migration
  and security reviews and Store/IARC completion remain separate strict holds.
