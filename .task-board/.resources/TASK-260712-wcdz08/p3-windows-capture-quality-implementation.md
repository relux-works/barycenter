# P3 Windows capture-quality engineering implementation

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-wcdz08`

Status: best-effort engineering path implemented; signed hardware and acoustic evidence not run

## Selected path

The shared Windows WASAPI helper now requests `AudioCategory_Communications`
through the public `IAudioClient2::SetClientProperties` API before format
negotiation and activation. Recorded clips, local self-test and live PTT all use
the same quality request and state machinery. The existing core capture ABI
remains frozen; quality configuration and results use a separately negotiated
versioned extension.

Communications category activation is telemetry, not evidence that Windows
inserted working AEC or noise suppression. The native helper therefore reports
`nativeEffectsVerified=false` in production and contains no path that promotes
that flag. Only a separately verified backend seam can make a route eligible
for `accepted`; the production route resolver is also deliberately `unknown`.

After native capture, the common product-owned safety stage targets -20 dBFS
RMS, limits digital gain to +12 dB, limits gain movement to 3 dB/s, and applies
the distinct -3 dBFS input ceiling last. Interleaved channel count is included
in slew timing. It does not modify the receiver graph, whose independent -1
dBFS post-mix output ceiling remains unchanged.

## Honest route policy

| Requested/resolved path | Engineering state |
| --- | --- |
| headphone + independently verified native effects | code-eligible for `accepted`; signed hardware proof still required |
| native effects not independently verified | `degraded/aec_unavailable`; bounded input stage remains active |
| built-in speaker with verified native effects | `degraded/reference_unavailable` until reference alignment is measured |
| unknown or ambiguous output | `degraded/route_unknown` |
| explicit requested route differs from resolved route | `degraded/route_excluded` |
| legacy unprocessed clip or self-test | `degraded/user_selected_unprocessed`; capability is not advertised |

Degraded clip or self-test processing requires explicit per-session consent.
Live PTT defaults to no degraded consent and fails closed with
`capture_quality_unsupported`, closing the native stream before any sample can
be encoded. A caller can explicitly consent to a degraded live session, which
remains visibly degraded for its whole generation.

## Lifecycle and state propagation

Each start creates a fresh quality generation and publishes `preparing`, then
`capturing`. Permission and device failures publish content-free typed states.
Stop publishes `stopping`, stops and closes the native capture, clears the
active generation and finally clears quality state. Existing services retain
their tested release, cancel, permission, device-loss, suspend, lock,
disconnect and quit teardown ownership.

Quality state is forwarded through the shared recording workflow, local
self-test and live sender. Production still does not advertise
`capture_quality_v1`; advertising remains gated on the later UI, integrated
regression and manual acceptance tasks.

## Realtime and persistence boundary

The existing native event-driven WASAPI capture thread continues to use its
preallocated buffers. Go capture workers apply the bounded stage before
resampling or encoding. The live worker has static exclusions for file I/O,
logging, media-store access and transport calls. No live samples or diagnostic
audio are persisted; recorded clips and self-test keep their existing explicit
user-owned draft behavior.

## Automated coverage and manual boundary

Repository tests cover gain, slew and final-ceiling bounds; honest route
decisions; fresh generations; ABI layout and extension negotiation; the public
communications-category request; the invariant that the native helper cannot
self-verify effects; shared clip/self-test propagation; typed live fail-closed
behavior; explicit degraded consent; existing teardown cases; race tests; and
Windows amd64/arm64 cross-builds.

The local macOS environment cannot compile or execute the native Windows C++
helper. Its build and packaging are checked by hosted Windows CI. Neither that
CI nor these deterministic tests prove an acoustic render-reference path,
native AEC/NS effectiveness, signed-AppContainer behavior, physical routes,
CPU/latency, double-talk preservation or listening quality. Those remain
`not-run` in `EPIC-260714-th54l3` and must be tested manually on supported
signed Windows builds and real hardware.
