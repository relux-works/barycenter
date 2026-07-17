# P3 capture-quality contract v1

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-1gmsvh`

Status: contract frozen; production disabled; implementation and C3 evidence pending

The normative machine-readable definitions are
[`protocol/capture-quality-v1.json`](../../protocol/capture-quality-v1.json) and
[`acceptance/phase3/capture-quality-contract-v1.json`](../../acceptance/phase3/capture-quality-contract-v1.json).
This decision does not implement AEC, noise suppression or AGC, does not add a
runtime capability, and records no acoustic, hardware or blinded-listening pass.

## 1. One processor, three workflows

Recorded clips, the five-second local record-then-play self-test and live PTT
must consume the output of the same reusable processor. Platform code may adapt
native devices and selected DSP APIs, but it may not implement three independent
effect chains or feed a workflow directly from raw capture while calling the
result accepted.

The order is fixed:

1. enter a visible and accessible `preparing` indication and allocate a fresh
   generation;
2. capture timestamped device-native PCM;
3. align the capture clock and convert to 48 kHz mono float PCM;
4. align an eligible render reference to the capture clock;
5. run AEC;
6. run noise suppression;
7. run bounded input AGC;
8. feed exactly one workflow sink: draft writer, local self-test writer, or
   live encoder;
9. close the sink, processor, reference and device before clearing the
   indicator.

The capture and render callbacks only timestamp and enqueue into preallocated,
bounded structures. The processor worker owns DSP state and the sink worker owns
file I/O or encoding. Allocation, blocking locks, file/network I/O and logging
are forbidden in both realtime callbacks.

## 2. Render-reference ownership and timing

The local playback graph owns the reference. It taps the exact samples submitted
to the output device after program, overlay, interrupt and received-live summing,
after the existing `-1 dBFS` post-mix limiter and after final non-amplifying local
volume. The tap is immediately before device submission. It therefore describes
what the local application actually attempts to render; the coordinator never
constructs or selects it.

Every reference block carries render-device sample position, monotonic host time
and route generation. The processor worker converts it to the 48 kHz mono
capture domain and searches at most 250 ms of delay. Reference older than 100 ms,
clock drift beyond 200 ppm, a discontinuity or a generation mismatch stops sample
commit and enters `reconfiguring`. Reference memory is capped at 500 ms and is
never persisted, uploaded, logged or included in crash reports.

Recording cues remain safe: the start cue completes before microphone samples
can be committed, and the stop cue starts only after capture closes. If a cue is
present in the render graph while commit is disabled, it may be in the reference;
it must never become microphone evidence.

## 3. Routes and honest state

The requested route vocabulary is `auto`, `speaker`, `headphone`; the resolved
vocabulary is `speaker`, `headphone`, `unknown`. `auto` is resolved before each
capture. Unknown, AirPlay, aggregate-device and remote-desktop routes are never
accepted by assumption.

An accepted speaker route requires an eligible synchronized reference plus
active AEC, NS and AGC. An accepted positively identified headphone route
requires active NS and AGC; AEC may be `not_required` because acoustic coupling
is not the speaker case. `not_required` must not be used for an unknown route.

The overall quality state is one of `accepted`, `degraded`, `unsupported`.
Individual effects are `active`, `not_required`, `unavailable` or `faulted`.
Input health is categorical and content-free: `ok`, `silent`, `too_quiet`,
`clipping`, `no_device`, `permission_denied`, `reference_stale`, `clock_unstable`
or `processor_overrun`.

On device or route change the client stops draft commit and live send, keeps the
indicator visible, advances generation, drains all old queues and re-resolves.
It must re-arm within 1500 ms or terminate as unsupported; it never resumes raw
samples from the old generation.

## 4. Fallback and mixed versions

Accepted capture can start normally. Degraded capture states the exact missing
effect and reason and needs a fresh, local, per-session confirmation before the
first microphone sample is committed. Unsupported capture commits nothing.

Recorded clips and self-test may offer a separately labelled unprocessed
Phase-1 action. It remains degraded and cannot advertise `capture_quality_v1`.
Live PTT is cancelled when unsupported; the UI may offer the existing recorded
clip action, but cannot switch automatically. A peer that does not advertise the
new capability is shown as unknown quality, never inferred accepted. Ordinary
`live_ptt_v1` interoperability remains a separate transport decision.

Rollback first withdraws the capability, then disables any quality-required
policy and terminates active quality generations. Additive unknown heartbeat
fields remain safe for old clients. Existing clip/live paths retain their own
explicit indicator and truthful Phase-1 claims.

## 5. Two ceilings

The input AGC target is `-20 dBFS RMS ±3 dB`; input peaks may not exceed
`-3 dBFS`. Digital gain is bounded to 12 dB and may change by at most 3 dB/s.
This ceiling is applied before every workflow sink and cannot be modified by the
coordinator.

The receiver output ceiling is different: the existing playback graph applies a
`-1 dBFS` post-mix limiter, followed only by final non-amplifying recipient-local
volume. It is controlled by the recipient and applied after all playback
branches. Input AGC cannot change or override it.

## 6. Protocol, heartbeat and history decision

`capture_quality_v1` is an additive register capability, but this contract task
does not advertise it. Later clients may advertise only after the common
processor, local state surface and exact-build deterministic suites are ready.
The capability never authorizes remote capture, coordinator-selected routes or
ceilings, or an acoustic/hardware claim.

Heartbeat gains one optional `capture_quality` object with these required fields:

`contract`, `generation`, `workflow`, `requested_mode`, `resolved_mode`,
`lifecycle`, `quality`, `aec`, `ns`, `agc`, `input_health`, `reason`,
`input_ceiling_dbfs`, `updated_monotonic_ms`.

`reference_age_ms` and `processor_overruns` are optional. Audio/reference bytes,
device identity, paths, filenames, transcripts and raw level samples are
forbidden. This is observational client state only.

No new persistent capture-quality history object is added. Existing terminal
receipts may use the content-free codes `capture_quality_unsupported`,
`capture_quality_degraded_declined`, `capture_quality_reconfigured` and
`capture_quality_failed`. Transient route, health and effect snapshots are not
stored in transmission history.

## 7. C3 rubric

Every accepted platform/route/workflow cell passes independently; averaging a
Windows failure into a macOS success, or a speaker failure into headphone
success, is forbidden. The frozen objective gates include:

- far-end-only: after 2 s convergence, median ERLE at least 18 dB, p10 at least
  10 dB, residual RMS no higher than `-45 dBFS`;
- near-end-only: absolute level change at most 3 dB and STOI delta at least
  `-0.05`;
- double-talk: near-end attenuation at most 3 dB, STOI delta at least `-0.05`
  and median ERLE at least 6 dB;
- noise suppression: SNR improvement at least 6 dB without more than 3 dB
  near-end attenuation;
- clipping: no more than 0.1% clipped samples and all AGC ceiling/gain/slew
  bounds respected;
- processor: added p95 latency at most 20 ms and zero callback allocations or
  blocking waits;
- route change: zero committed samples during reconfiguration and re-arm in at
  most 1500 ms or an explicit terminal result;
- clock drift: exercise ±200 ppm and keep accepted reference age at most 100 ms.

The matrix covers far-end-only, near-end-only, double-talk, echo-path change,
route change, clock drift, clipping, too-quiet, silence, device loss, processor
overrun, missing reference and effect failure for all three workflows, with an
explicit platform/route rollup.

The listening method uses at least three independent listeners and two
randomized, hash-named repetitions per cell. Platform, route and processing
labels are hidden. “Intelligible return echo” means correctly transcribing two
consecutive far-end words as returned echo. An accepted cell needs zero such
findings across six ratings, median near-end quality at least 4/5 and no
double-talk rating below 3/5. Degraded results are published separately.

Those are acceptance targets, not current results. Signed Windows/macOS hardware,
the acoustic matrix, blinded listening, accessibility/indicator review and
physical resource measurements are all `not-run` and belong to manual epic
`EPIC-260714-th54l3`, including `TASK-260712-2e80pr`.

## 8. Downstream ownership

- `TASK-260712-265o0f` and `TASK-260712-2gaswa` prove exact platform paths on
  real signed builds; they may narrow accepted routes but not weaken this rubric.
- `TASK-260712-1pw1l1` implements the additive state vocabulary and surfaces.
- `TASK-260712-39czd2` creates deterministic fixtures and calculations.
- `TASK-260712-wcdz08` and `TASK-260712-2egweh` implement platform processors.
- `TASK-260712-1023d7` runs integrated repository regressions.
- `TASK-260712-2e80pr` records the real acoustic/blinded C3 matrix in the manual
  test epic.
