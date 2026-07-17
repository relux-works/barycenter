# P3 capture diagnostics capability surface v1

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-1pw1l1`

Status: additive protocol and presentation seam implemented; production
capability unadvertised; platform DSP and manual evidence not run

This handoff implements the observational surface frozen by
[`capture-quality.v1`](../../protocol/capture-quality-v1.json). Its shared
malformed and ordering fixtures are
[`capture-quality-v1-vectors.json`](../../protocol/capture-quality-v1-vectors.json),
and the repository-only decision record is
[`capture-diagnostics-capability-v1.json`](../../acceptance/phase3/capture-diagnostics-capability-v1.json).
It does not implement AEC, NS or AGC and does not claim a working physical
route, acoustic quality, signed-package result or production capability.

## Wire and ownership

`state.capture_quality` is optional and additive. The canonical coordinator Go
model and the Windows Go mirror are byte-checked together; Swift implements the
same field names, enums and relational validation. Strict and tolerant codecs
both reject a present malformed object. Absence remains the only legacy shape.

A client may send the object only on a connection whose canonical register set
contains `capture_quality_v1`. The coordinator rejects a claim without that
capability, validates monotonically increasing `(generation,
updated_monotonic_ms)` values per authenticated socket and clears the snapshot
and guard at reconnect. Stale state never reaches consumers. Snapshots are
ephemeral defensive copies and are not written to transmission history.

The object is observational. It has no command type, start flag, route setter,
permission action or resume action. Receiving it cannot open, arm, reconfigure
or resume a microphone.

## Shared presentation semantics

Windows and macOS expose the same pure guidance projection for later native UI
tasks:

- no capability or no state: `unsupported`, `mixed_version`,
  `capture_quality.mixed_version`;
- unhealthy input: `capture_quality.input.<input_health>`;
- otherwise: `capture_quality.<quality>.<reason>`.

An available presentation carries requested/resolved route, AEC/NS/AGC state,
input health, the `-3 dBFS` capture-input ceiling and the separate `-1 dBFS`
recipient-output ceiling. An unavailable presentation leaves these details
empty, so mixed-version status cannot manufacture route or effect parity.

This is deliberately a semantic seam, not a production UI claim. A legacy node
continues ordinary heartbeats and transport but can never be labelled accepted.
Unsupported or degraded state remains visible as such; later UI work may
localize the stable keys without inventing platform parity.

## Diagnostics and privacy

The only structured diagnostic projection contains categorical contract,
generation, workflow, quality, reason and input-health values, plus the bounded
processor-overrun counter when present. It excludes audio, raw levels, render
reference, device identity, filenames, paths, transcripts and secrets. Neither
platform logs accepted/healthy heartbeats; degraded and unhealthy transitions
use only this content-free projection.

## Rollout boundary

Both clients withhold `capture_quality` from a heartbeat unless the exact build
advertises `capture_quality_v1`. Neither production capability list advertises
it in this change: production builds still do not advertise the surface. The
later platform processor and integrated-regression tasks
must populate this seam and pass deterministic gates before advertisement may
be considered. Real-app, signed-build, physical-device and acoustic testing
remains `not-run` in `EPIC-260714-th54l3`.

Rollback is additive: withdraw or omit the capability and the extension, clear
the ephemeral coordinator snapshot on reconnect, and retain the existing
clip/live heartbeat behavior.
