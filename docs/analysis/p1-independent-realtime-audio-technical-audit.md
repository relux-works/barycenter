# Phase 1 realtime-audio technical audit

- Date: 2026-07-15
- Task: `TASK-260712-1uz0za`
- Frozen review base: `aed5d7e5225aca0d4d5b0ad8347cfd500f6c0dac`
- Review mode: rigorous inline self-audit with corrective patch
- Acceptance state: technical audit complete; independent and hardware signoff open

## Independence and evidence boundary

The same strict inline execution chain implemented some of the audio paths now
under review. This report therefore does **not** claim that its reviewer was
independent. It is a reproducible technical packet for a separate audio
reviewer and keeps checklist item 1 open.

The packet covers deterministic scheduling, callback source guards, races,
failure/cancellation state machines, maximum prepared memory and repeated
operation. It cannot establish audible clipping, pumping, route noise, real
provider drift, packaged-app behavior or physical 200/500 ms timing. Those
observations remain in manual epic `EPIC-260714-th54l3`, principally
`TASK-260712-2hodti`.

## Reviewed paths and invariants

The review traced the `prepare -> arm -> first sample -> end/cancel/fail ->
dispose` lifecycle through `MediaClipClient`, both platform mixers, the Windows
portable render engine and the macOS AVAudioEngine graph. It re-derived these
invariants from the P1 contract and root-review amendments:

| Seam | Re-derived invariant and automated evidence |
| --- | --- |
| Render boundary | Windows rejects allocation, locks, waits, sleeps and goroutine creation in the render call graph and measures zero active-overlay allocations. macOS source guards reject dispatch, locks, allocation, I/O and waits inside the marked callback. |
| Gain and limiter | Both paths keep music/clip gain before a post-mix limiter and local master gain after it. Deterministic fixtures hit the frozen local ceiling without bypass. Audible distortion remains a listening assertion. |
| Pre-duck/start | Overlay pre-duck is a 250 ms raised-cosine ramp with deterministic late catch-up. Interrupt freezes the main ring at T instead of consuming it silently. Callback timestamps stay inside the synthetic 200/500 ms bounds. |
| Anchor/resume | Interrupt anchor is provider position minus queued ring duration, clamped at zero. A generation owns at most one suspend token and one seek/resume or explicit abandon. |
| Cancellation/delete | Active cancellation fades once, releases the render owner, acknowledges once and honors `resume_main`; late callbacks cannot turn cancellation into success. |
| Lifetime/memory | The 180-second maximum retains one prepared stereo PCM buffer below 64 MiB. Both platforms execute 100 sequential overlays without stale owners, deadlock or prohibited retained growth. |
| Control concurrency | macOS reader state is atomic, multi-producer gain publication is serialized outside the callback, FIFO idle is interruptible, and heartbeat state is one queue-owned snapshot. Windows race tests cover player/ring/mixer cancellation and natural-completion overlap. |

## Findings

### P1-AUDIO-001 — HIGH — closed

**Finding.** The Windows mixer had no asynchronous failure callback. A failed
interrupt provider resume could consequently emit `media_ended/completed`.
Cancellation also ignored `CancelMediaPayload.ResumeMain` and always resumed
the provider. A cancel racing natural resume could attempt a second finalizer
or acknowledge the wrong result.

**Correction.** `MediaClipMixer.Arm` now has a typed asynchronous failure path.
The interrupt finalizer consumes and caches one anchor/render-owner outcome;
concurrent cancellation waits for that outcome. `resume_main=false` abandons
the token while keeping the provider paused, and resume failure emits
`media_failed/audio_graph_failed`, never `media_ended`.

**Re-review evidence.** New client and mixer regressions cover failures before
and after start, natural resume failure, no-resume cancellation and cancellation
during a gated natural resume. Full Windows `go test -race -count=1 ./...`
passes.

### P1-AUDIO-002 — HIGH — closed

**Finding.** macOS shared `readerActive` as an unsynchronized raw `Bool` between
the reader and control threads. Its gain-command ring was implemented as SPSC,
but PlayerCore, overlay/interrupt control and recovery could publish from
multiple queues, allowing commands to overwrite one another.

**Correction.** Reader ownership now uses the existing atomic primitive. Gain
publishers serialize their fixed-buffer head update with an `NSLock` outside
the render boundary; the single render consumer remains lock-free and
allocation-free.

**Re-review evidence.** Source-boundary tests require the atomic state and
producer serialization and prove the lock is absent from the marked render
callback. The complete 218-test Swift suite and focused 100-overlay/maximum
buffer cases pass.

### P1-AUDIO-003 — HIGH — closed

**Finding.** A macOS FIFO reader could block forever in `open(O_RDONLY)` when no
writer connected, so `stopEngine()` could not bound shutdown. Separately,
heartbeat construction read queue-owned playback, anchor and speaker fields
without the PlayerCore queue, creating an inconsistent/data-racy snapshot.

**Correction.** FIFO open/read is non-blocking with bounded idle polling while
preserving ring-full backpressure and zero dropping. Player playback fields are
private queue-owned state, and heartbeat construction is one `queue.sync`
snapshot.

**Re-review evidence.** Source regressions pin the interruptible FIFO open and
the synchronized heartbeat boundary. Full Swift tests pass after the change.

No other critical or high technical finding remains in this inline audit.

## Verification executed

- Windows complete portable suite under the race detector: passed.
- macOS focused render/mixer suite: 16 tests passed, including 100 overlays,
  maximum P1 memory, resume failure and cancellation overlap.
- macOS complete package suite: 218 tests in 35 suites passed.
- Repository-wide clean exact-head acceptance and hosted CI are recorded on the
  task after the corrective revision is frozen.

## Required independent and manual signoff

A non-implementing audio reviewer must still:

1. diff the frozen base and corrective revision against the P1 audio contract;
2. inspect both render boundaries, ownership transitions and all three closed
   high findings;
3. rerun the deterministic and race commands from this packet;
4. consume the A3/A4 real-app evidence from `TASK-260712-2hodti`, including
   listening, routes, packaged apps and physical 200/500 ms measurements;
5. record identity, revision and approval without reusing this inline audit as
   independent signoff.

Until those steps are complete, the original review task remains open even
though best-effort engineering may continue in strict plan order.
