# P3 macOS capture-quality engineering implementation

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-2egweh`

Status: best-effort engineering path implemented; physical and acoustic evidence not run

## Selected path

The macOS capture backend uses the public `AVAudioInputNode` voice-processing
mode available on the supported deployment target. When quality processing is
requested it enables voice processing, disables bypass, enables native AGC and
then applies the product-owned bounded input safety stage after conversion to
48 kHz mono. The same backend is selected as `recorded_clip`,
`local_self_test`, or `live_ptt`; there are no per-workflow DSP forks.

The safety stage targets -20 dBFS RMS, limits digital gain to +12 dB, limits
gain movement to 3 dB/s, and applies the distinct -3 dBFS input ceiling last.
It does not touch the receiver graph, whose separate -1 dBFS post-mix ceiling
remains unchanged.

## Realtime boundary

The Core Audio tap downmixes into a fixed 16,384-sample ring guarded by a
nonblocking `tryLock`. It signals pre-created dispatch sources and performs no
resampling, quality-state callback, file/network I/O, encoding, or client
callback. The backend serial worker drains the ring, resamples, applies the
input safety stage, and invokes the selected workflow. Contention, overflow,
unsupported channel count, or missing float data terminates capture with a
content-free `processor_overrun` state.

No live samples or diagnostic audio are persisted. Recorded clips and the
local self-test continue to use their existing explicitly user-owned draft
stores.

## Honest route policy

| Requested/resolved path | Engineering state |
| --- | --- |
| positively identified headphone + native voice processing | eligible for `accepted` in code; physical proof still required |
| built-in speaker + native voice processing | `degraded/reference_unavailable` because the API does not expose reference age |
| unknown, AirPlay, aggregate, remote, or ambiguous output | `degraded/route_unknown` |
| explicit requested route differs from resolved route | `degraded/route_excluded` |
| native voice processing unavailable | `degraded/aec_unavailable` |
| legacy unprocessed clip or self-test | `degraded/user_selected_unprocessed`; capability is not advertised |

Degraded clip or self-test processing requires fresh per-session local consent.
Live PTT defaults to no degraded consent and fails closed with
`capture_quality_unsupported`; it never falls through to raw live samples.
Production still does not advertise `capture_quality_v1`.

## Lifecycle and state propagation

Each backend start creates a fresh quality generation and publishes
`preparing`, then `capturing`. Configuration changes publish `reconfiguring`
and terminate the old capture. Stop publishes `stopping`, removes the tap and
observer, cancels both dispatch sources, clears the mailbox, resets DSP and
resampling state, releases the audio engine, and finally clears the quality
state. Existing capture owners cover user release/cancel, permission loss,
device loss, sleep, lock, disconnect and quit.

Quality state is forwarded through the shared clip/self-test controller and the
live sender/node. The app runtime can present it, but it remains absent from
advertised capabilities until the later UI, integrated-regression, and manual
acceptance tasks are complete.

## Automated coverage and manual boundary

Repository tests cover input ceiling, gain and slew bounds, route decision
truthfulness, generation freshness, exact workflow selection, quality-state
forwarding, typed live rejection, 100-cycle live teardown, all existing capture
terminal paths, and static realtime-tap exclusions.

Native VPIO does not expose an offline synthetic render-reference injection or
reference-age measurement. Therefore this change does not claim deterministic
AEC/NS C3 acceptance, signed-app behavior, a physical speaker or headphone
pass, acoustic echo results, CPU/latency measurements, Bluetooth or external
interface behavior, or blinded listening. Those remain `not-run` in
`EPIC-260714-th54l3`; the checked-in capture-quality harness and later
integrated task consume the platform evidence when it exists.
